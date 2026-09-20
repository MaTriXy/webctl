package jev

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dorkitude/multi_search_web/internal/provider"
)

// jevServer is a mock System One endpoint. handler receives the decoded
// request and returns the response payload (or an HTTP status to fail with).
type jevServer struct {
	srv     *httptest.Server
	calls   atomic.Int32
	handler func(req SystemOneRequest) (status int, body any)

	mu       sync.Mutex // guards lastReq/lastAuth (handlers run concurrently)
	lastReq  SystemOneRequest
	lastAuth string
}

// last returns the most recent request and auth header.
func (js *jevServer) last() (SystemOneRequest, string) {
	js.mu.Lock()
	defer js.mu.Unlock()
	return js.lastReq, js.lastAuth
}

func newJevServer(t *testing.T, handler func(req SystemOneRequest) (int, any)) *jevServer {
	t.Helper()
	js := &jevServer{handler: handler}
	js.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		js.calls.Add(1)
		if r.URL.Path != systemOnePath || r.Method != http.MethodPost {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		raw, _ := io.ReadAll(r.Body)
		var req SystemOneRequest
		if err := json.Unmarshal(raw, &req); err != nil {
			t.Errorf("bad request JSON: %v\n%s", err, raw)
		}
		js.mu.Lock()
		js.lastAuth = r.Header.Get("Authorization")
		js.lastReq = req
		js.mu.Unlock()
		status, body := handler(req)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		switch b := body.(type) {
		case string:
			_, _ = io.WriteString(w, b)
		default:
			_ = json.NewEncoder(w).Encode(b)
		}
	}))
	t.Cleanup(js.srv.Close)
	return js
}

func (js *jevServer) client() *Client {
	c := NewClient("test-key")
	c.BaseURL = js.srv.URL + "/"
	c.sleep = func(context.Context, time.Duration) error { return nil }
	return c
}

func f64(v float64) *float64 { return &v }

// scoreAnswer builds a raw score answer with the given probability mass over levels.
func scoreAnswer(probs ...float64) Answer {
	a := Answer{Type: TypeScore, Probabilities: map[string]float64{}, Legend: map[string]string{}}
	var expected, best float64
	for i, p := range probs {
		k := fmt.Sprint(i)
		a.Probabilities[k] = p
		a.Legend[k] = "level " + k
		expected += float64(i) * p
		if p > best {
			best = p
		}
	}
	a.Score = f64(expected)
	a.Confidence = f64(best)
	return a
}

func noulAnswer(p float64) Answer { return Answer{Type: TypeNoul, Noul: f64(p)} }

// answerAll responds to every question with the same score answer.
func answerAll(ans Answer) func(SystemOneRequest) (int, any) {
	return func(req SystemOneRequest) (int, any) {
		resp := SystemOneResponse{Model: req.Model, Answers: map[string]Answer{}, Usage: Usage{InputTokens: 10, OutputTokens: 1}}
		for k := range req.Questions {
			resp.Answers[k] = ans
		}
		return 200, resp
	}
}

func TestSystemOneRequest(t *testing.T) {
	js := newJevServer(t, answerAll(noulAnswer(0.9)))
	c := js.client()
	resp, err := c.SystemOne(context.Background(), &SystemOneRequest{
		State:     map[string]any{"text": "hi"},
		Questions: map[string]Question{"q": NoulQuestion("Is it a greeting?")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if js.lastAuth != "Bearer test-key" {
		t.Errorf("Authorization = %q", js.lastAuth)
	}
	if js.lastReq.Model != DefaultModel {
		t.Errorf("model = %q, want default %q", js.lastReq.Model, DefaultModel)
	}
	if resp.Usage.InputTokens != 10 || resp.Answers["q"].Noul == nil || *resp.Answers["q"].Noul != 0.9 {
		t.Errorf("resp = %+v", resp)
	}

	c.Model = "jev-custom"
	if _, err := c.SystemOne(context.Background(), &SystemOneRequest{State: 1, Questions: map[string]Question{"q": NoulQuestion("x")}}); err != nil {
		t.Fatal(err)
	}
	if js.lastReq.Model != "jev-custom" {
		t.Errorf("model = %q, want jev-custom", js.lastReq.Model)
	}
}

func TestSystemOneValidation(t *testing.T) {
	c := NewClient("")
	if _, err := c.SystemOne(context.Background(), &SystemOneRequest{Questions: map[string]Question{"q": NoulQuestion("x")}}); err == nil || !strings.Contains(err.Error(), "API key is empty") {
		t.Errorf("empty key: %v", err)
	}
	c = NewClient("k")
	if _, err := c.SystemOne(context.Background(), nil); err == nil {
		t.Error("nil request should error")
	}
	if _, err := c.SystemOne(context.Background(), &SystemOneRequest{}); err == nil || !strings.Contains(err.Error(), "no questions") {
		t.Errorf("no questions: %v", err)
	}
}

func TestSystemOneUnauthorizedNoRetry(t *testing.T) {
	js := newJevServer(t, func(SystemOneRequest) (int, any) { return 401, `{"error":"bad key"}` })
	err := js.client().Validate(context.Background())
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Status != 401 || apiErr.Body != "bad key" {
		t.Fatalf("err = %v", err)
	}
	if !IsUnauthorized(err) || !apiErr.Unauthorized() {
		t.Error("expected unauthorized")
	}
	if !strings.Contains(err.Error(), "JEV_API_KEY") {
		t.Errorf("error should hint at JEV_API_KEY: %v", err)
	}
	if js.calls.Load() != 1 {
		t.Errorf("401 should not be retried, got %d calls", js.calls.Load())
	}
}

func TestSystemOneRetriesTransient(t *testing.T) {
	var n atomic.Int32
	js := newJevServer(t, func(req SystemOneRequest) (int, any) {
		if n.Add(1) < 3 {
			return 503, `{"message":"try later"}`
		}
		return answerAll(noulAnswer(0.7))(req)
	})
	c := js.client()
	var slept []time.Duration
	c.sleep = func(_ context.Context, d time.Duration) error { slept = append(slept, d); return nil }
	if err := c.Validate(context.Background()); err != nil {
		t.Fatalf("expected success after retries, got %v", err)
	}
	if js.calls.Load() != 3 {
		t.Errorf("calls = %d, want 3", js.calls.Load())
	}
	if len(slept) != 2 || slept[0] != 500*time.Millisecond || slept[1] != time.Second {
		t.Errorf("backoff = %v, want [500ms 1s]", slept)
	}
}

func TestSystemOneGivesUpAfterMaxAttempts(t *testing.T) {
	js := newJevServer(t, func(SystemOneRequest) (int, any) { return 429, `rate limited` })
	err := js.client().Validate(context.Background())
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Status != 429 {
		t.Fatalf("err = %v", err)
	}
	if js.calls.Load() != maxAttempts {
		t.Errorf("calls = %d, want %d", js.calls.Load(), maxAttempts)
	}
	if !strings.Contains(err.Error(), "rate limited") {
		t.Errorf("error text = %v", err)
	}
}

func TestSystemOneNoRetry(t *testing.T) {
	js := newJevServer(t, func(SystemOneRequest) (int, any) { return 500, `` })
	c := js.client()
	c.NoRetry = true
	if err := c.Validate(context.Background()); err == nil {
		t.Fatal("expected error")
	}
	if js.calls.Load() != 1 {
		t.Errorf("NoRetry should make exactly 1 call, got %d", js.calls.Load())
	}
}

func TestSystemOneRetryStopsOnCancelledContext(t *testing.T) {
	js := newJevServer(t, func(SystemOneRequest) (int, any) { return 500, `` })
	c := js.client()
	ctx, cancel := context.WithCancel(context.Background())
	c.sleep = func(ctx context.Context, _ time.Duration) error { cancel(); return ctx.Err() }
	err := c.Validate(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
	if js.calls.Load() != 1 {
		t.Errorf("calls = %d, want 1", js.calls.Load())
	}
}

func TestSystemOneDecodeError(t *testing.T) {
	js := newJevServer(t, func(SystemOneRequest) (int, any) { return 200, `{not json` })
	err := js.client().Validate(context.Background())
	if err == nil || !strings.Contains(err.Error(), "decode response") {
		t.Errorf("err = %v", err)
	}
}

func TestRetryable(t *testing.T) {
	if retryable(&APIError{Status: 400}) || retryable(&APIError{Status: 401}) {
		t.Error("4xx (other than 429) should not retry")
	}
	if !retryable(&APIError{Status: 429}) || !retryable(&APIError{Status: 502}) {
		t.Error("429/5xx should retry")
	}
	if retryable(context.Canceled) || retryable(fmt.Errorf("wrap: %w", context.DeadlineExceeded)) {
		t.Error("context errors should not retry")
	}
	if !retryable(errors.New("connection reset")) {
		t.Error("network errors should retry")
	}
}

var (
	res1 = provider.SearchResult{Title: "Attention Is All You Need", URL: "https://arxiv.org/abs/1706.03762", Snippet: "We propose the Transformer."}
	res2 = provider.SearchResult{Title: "SEO blog", URL: "https://content-farm.example/attention", Snippet: "Top 10 attention tips."}
)

func TestScoreResult(t *testing.T) {
	js := newJevServer(t, answerAll(scoreAnswer(0.01, 0.02, 0.15, 0.82)))
	ans, err := js.client().ScoreResult(context.Background(), "transformers", res1, nil)
	if err != nil {
		t.Fatal(err)
	}
	q, ok := js.lastReq.Questions["relevance"]
	if !ok {
		t.Fatalf("questions = %v", js.lastReq.Questions)
	}
	if q.Type != TypeScore || len(q.Criteria) != 4 {
		t.Errorf("question = %+v", q)
	}
	if !strings.Contains(q.Instructions, "Query: transformers") || !strings.Contains(q.Instructions, res1.Title) {
		t.Errorf("instructions should embed query and result:\n%s", q.Instructions)
	}
	state, _ := js.lastReq.State.(map[string]any)
	if state["query"] != "transformers" {
		t.Errorf("state = %v", state)
	}
	if got := ans.Score; got < 2.7 || got > 2.9 {
		t.Errorf("score = %v", got)
	}
	if ans.Confidence != 0.82 || ans.MaxScore() != 3 || ans.Level() != 3 {
		t.Errorf("ans = %+v", ans)
	}
	if got := ans.FormatProbabilities(); got != "{0: 0.01, 1: 0.02, 2: 0.15, 3: 0.82}" {
		t.Errorf("FormatProbabilities = %q", got)
	}
}

func TestScoreResultCustomRubric(t *testing.T) {
	js := newJevServer(t, answerAll(scoreAnswer(0.5, 0.5)))
	rubric := []string{"no", "yes"}
	if _, err := js.client().ScoreResult(context.Background(), "q", res1, rubric); err != nil {
		t.Fatal(err)
	}
	got := js.lastReq.Questions["relevance"].Criteria
	if strings.Join(got, ",") != "no,yes" {
		t.Errorf("criteria = %v, want custom rubric", got)
	}
}

func TestScoreResultErrors(t *testing.T) {
	// Missing answer key.
	js := newJevServer(t, func(SystemOneRequest) (int, any) {
		return 200, SystemOneResponse{Answers: map[string]Answer{"other": scoreAnswer(1)}}
	})
	if _, err := js.client().ScoreResult(context.Background(), "q", res1, nil); err == nil || !strings.Contains(err.Error(), `missing "relevance"`) {
		t.Errorf("err = %v", err)
	}
	// Wrong answer type.
	js = newJevServer(t, answerAll(noulAnswer(0.5)))
	if _, err := js.client().ScoreResult(context.Background(), "q", res1, nil); err == nil || !strings.Contains(err.Error(), "expected score answer") {
		t.Errorf("err = %v", err)
	}
}

func TestScoreBatch(t *testing.T) {
	js := newJevServer(t, func(req SystemOneRequest) (int, any) {
		resp := SystemOneResponse{Answers: map[string]Answer{
			BatchKey(0): scoreAnswer(0, 0, 0.1, 0.9),
			BatchKey(1): scoreAnswer(0.8, 0.2, 0, 0),
		}}
		return 200, resp
	})
	answers, err := js.client().ScoreBatch(context.Background(), "transformers", []SearchResult{res1, res2}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if js.calls.Load() != 1 {
		t.Errorf("batch should make 1 call, got %d", js.calls.Load())
	}
	if len(js.lastReq.Questions) != 2 {
		t.Errorf("questions = %d, want 2", len(js.lastReq.Questions))
	}
	if !strings.Contains(js.lastReq.Questions[BatchKey(1)].Instructions, `id "result_1"`) {
		t.Errorf("batch instructions should reference the result id:\n%s", js.lastReq.Questions[BatchKey(1)].Instructions)
	}
	state, _ := js.lastReq.State.(map[string]any)
	items, _ := state["results"].([]any)
	if len(items) != 2 || items[0].(map[string]any)["id"] != "result_0" {
		t.Errorf("state.results = %v", items)
	}
	if answers[BatchKey(0)].Score < 2.5 || answers[BatchKey(1)].Score > 0.5 {
		t.Errorf("answers = %+v", answers)
	}

	empty, err := js.client().ScoreBatch(context.Background(), "q", nil, nil)
	if err != nil || len(empty) != 0 {
		t.Errorf("empty batch = %v, %v", empty, err)
	}
}

func TestNoulResult(t *testing.T) {
	js := newJevServer(t, answerAll(noulAnswer(0.15)))
	ans, err := js.client().NoulResult(context.Background(), "q", "Is this a paper?", res2)
	if err != nil {
		t.Fatal(err)
	}
	q := js.lastReq.Questions["answer"]
	if q.Type != TypeNoul || len(q.Criteria) != 0 || !strings.Contains(q.Instructions, "Question: Is this a paper?") {
		t.Errorf("question = %+v", q)
	}
	if ans.Yes() {
		t.Error("P=0.15 should be no")
	}
	if c := ans.Confidence(); c < 0.69 || c > 0.71 {
		t.Errorf("confidence = %v, want 0.7", c)
	}
	if _, err := js.client().NoulResult(context.Background(), "q", "", res1); err == nil {
		t.Error("empty question should error")
	}
}

func TestNoulBatch(t *testing.T) {
	js := newJevServer(t, answerAll(noulAnswer(0.9)))
	answers, err := js.client().NoulBatch(context.Background(), "q", "Relevant?", []SearchResult{res1, res2})
	if err != nil {
		t.Fatal(err)
	}
	if len(answers) != 2 || !answers[BatchKey(0)].Yes() {
		t.Errorf("answers = %+v", answers)
	}
	if _, err := js.client().NoulBatch(context.Background(), "q", "", []SearchResult{res1}); err == nil {
		t.Error("empty question should error")
	}
}

func TestQualifyPerResult(t *testing.T) {
	js := newJevServer(t, func(req SystemOneRequest) (int, any) {
		state, _ := req.State.(map[string]any)
		result, _ := state["result"].(map[string]any)
		if result["url"] == res2.URL {
			return 500, `boom`
		}
		return answerAll(scoreAnswer(0, 0, 0.2, 0.8))(req)
	})
	c := js.client()
	c.NoRetry = true
	out, usage, err := c.Qualify(context.Background(), "q", []SearchResult{res1, res2}, QualifyOptions{Concurrency: 2})
	if err != nil {
		t.Fatalf("partial failure should not error: %v", err)
	}
	if len(out) != 2 || out[0].Result != res1 || out[1].Result != res2 {
		t.Fatalf("output misaligned: %+v", out)
	}
	if out[0].Score == nil || out[0].Err != nil {
		t.Errorf("out[0] = %+v", out[0])
	}
	if out[1].Score != nil || out[1].Err == nil {
		t.Errorf("out[1] should carry the error: %+v", out[1])
	}
	if usage.InputTokens != 10 {
		t.Errorf("usage = %+v (only the successful call should count)", usage)
	}
	if out[0].Value() < 2.5 || out[0].Max() != 3 || out[0].Confidence() != 0.8 {
		t.Errorf("qualified accessors = %v %v %v", out[0].Value(), out[0].Max(), out[0].Confidence())
	}
	if out[1].Value() != -1 || out[1].Max() != 0 || out[1].Confidence() != 0 {
		t.Errorf("unanswered accessors = %v %v %v", out[1].Value(), out[1].Max(), out[1].Confidence())
	}
}

func TestQualifyAllFail(t *testing.T) {
	js := newJevServer(t, func(SystemOneRequest) (int, any) { return 400, `bad` })
	_, _, err := js.client().Qualify(context.Background(), "q", []SearchResult{res1, res2}, QualifyOptions{})
	if err == nil || !strings.Contains(err.Error(), "failed for all 2 results") {
		t.Errorf("err = %v", err)
	}
}

func TestQualifyEmpty(t *testing.T) {
	js := newJevServer(t, answerAll(scoreAnswer(1)))
	out, _, err := js.client().Qualify(context.Background(), "q", nil, QualifyOptions{})
	if err != nil || len(out) != 0 || js.calls.Load() != 0 {
		t.Errorf("empty qualify: out=%v err=%v calls=%d", out, err, js.calls.Load())
	}
}

func TestQualifyNoul(t *testing.T) {
	js := newJevServer(t, answerAll(noulAnswer(0.8)))
	out, _, err := js.client().Qualify(context.Background(), "q", []SearchResult{res1}, QualifyOptions{Noul: "Is it a paper?"})
	if err != nil {
		t.Fatal(err)
	}
	if out[0].Noul == nil || out[0].Score != nil || out[0].Value() != 0.8 || out[0].Max() != 1 {
		t.Errorf("out = %+v", out[0])
	}
}

func TestQualifyBatchMissingAnswer(t *testing.T) {
	js := newJevServer(t, func(SystemOneRequest) (int, any) {
		return 200, SystemOneResponse{Answers: map[string]Answer{BatchKey(0): scoreAnswer(0, 1)}}
	})
	out, _, err := js.client().Qualify(context.Background(), "q", []SearchResult{res1, res2}, QualifyOptions{Batch: true})
	if err != nil {
		t.Fatal(err)
	}
	if js.calls.Load() != 1 {
		t.Errorf("batch should be one call, got %d", js.calls.Load())
	}
	if out[0].Score == nil || out[0].Err != nil {
		t.Errorf("out[0] = %+v", out[0])
	}
	if out[1].Score != nil || out[1].Err == nil || !strings.Contains(out[1].Err.Error(), "no answer") {
		t.Errorf("out[1] = %+v", out[1])
	}

	// Noul batch path.
	js = newJevServer(t, answerAll(noulAnswer(0.3)))
	out, _, err = js.client().Qualify(context.Background(), "q", []SearchResult{res1, res2}, QualifyOptions{Batch: true, Noul: "Relevant?"})
	if err != nil {
		t.Fatal(err)
	}
	if out[1].Noul == nil || out[1].Noul.Yes() {
		t.Errorf("out[1] = %+v", out[1])
	}
}

func TestQualifyBatchRequestError(t *testing.T) {
	js := newJevServer(t, func(SystemOneRequest) (int, any) { return 403, `forbidden` })
	_, _, err := js.client().Qualify(context.Background(), "q", []SearchResult{res1}, QualifyOptions{Batch: true})
	if !IsUnauthorized(err) {
		t.Errorf("err = %v", err)
	}
}

func TestAnswerConversions(t *testing.T) {
	if _, err := (Answer{Type: TypeScore}).AsScore(); err == nil {
		t.Error("score answer without score should error")
	}
	if _, err := (Answer{Type: TypeNoul}).AsNoul(); err == nil {
		t.Error("noul answer without noul should error")
	}
	s := &ScoreAnswer{Score: 1.6}
	if s.MaxScore() != 0 || s.Level() != 2 {
		t.Errorf("fallbacks: max=%d level=%d", s.MaxScore(), s.Level())
	}
	s = &ScoreAnswer{Probabilities: map[string]float64{"0": 0.5, "1": 0.5}}
	if s.Level() != 1 {
		t.Errorf("tie should pick the higher level, got %d", s.Level())
	}
	n := &NoulAnswer{Probability: 0.5}
	if !n.Yes() || n.Confidence() != 0 {
		t.Errorf("coin flip: yes=%v conf=%v", n.Yes(), n.Confidence())
	}
	var u Usage
	u.Add(Usage{InputTokens: 1, OutputTokens: 2})
	u.Add(Usage{InputTokens: 3, OutputTokens: 4})
	if u.InputTokens != 4 || u.OutputTokens != 6 {
		t.Errorf("usage = %+v", u)
	}
}

func TestDefaultRubric(t *testing.T) {
	r := DefaultRubric()
	if len(r) != 4 || !strings.HasPrefix(r[0], "Off-topic") || !strings.HasPrefix(r[3], "Best available") {
		t.Errorf("DefaultRubric = %v", r)
	}
	r[0] = "mutated"
	if DefaultRubric()[0] == "mutated" {
		t.Error("DefaultRubric must return a copy")
	}
}

func TestSummarize(t *testing.T) {
	long := strings.Repeat("x", 250)
	for in, want := range map[string]string{
		``:                          "",
		`{"error":"e"}`:             "e",
		`{"error":{"message":"m"}}`: "m",
		`{"detail":"d"}`:            "d",
		"plain\n  text":             "plain text",
		`{"error":"` + long + `"}`:  long[:200] + "…",
	} {
		if got := summarize([]byte(in)); got != want {
			t.Errorf("summarize(%q) = %q, want %q", in, got, want)
		}
	}
}
