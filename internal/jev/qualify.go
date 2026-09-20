package jev

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync"

	"github.com/dorkitude/smart_search/internal/prompts"
	"github.com/dorkitude/smart_search/internal/provider"
)

// SearchResult is re-exported so callers can use jev without importing provider.
type SearchResult = provider.SearchResult

// DefaultRubric is the built-in 0–3 relevance scale (from prompts/relevance-score.md).
func DefaultRubric() []string {
	return append([]string(nil), prompts.MustLoad(prompts.RelevanceScore).Criteria...)
}

// DefaultConcurrency bounds parallel per-result Jev calls.
const DefaultConcurrency = 8

// BatchKey returns the question key used for the i-th result in a batch request.
func BatchKey(i int) string { return "result_" + strconv.Itoa(i) }

// resultState is the structured JSON sent as Jev state for a single result.
type resultState struct {
	Query  string       `json:"query"`
	Result SearchResult `json:"result"`
}

type batchItem struct {
	ID string `json:"id"`
	SearchResult
}

type batchState struct {
	Query   string      `json:"query"`
	Results []batchItem `json:"results"`
}

// ScoreResult sends a single search result to Jev for relevance scoring.
// A nil or empty rubric uses the default 0–3 scale.
func (c *Client) ScoreResult(ctx context.Context, query string, result SearchResult, rubric []string) (*ScoreAnswer, error) {
	ans, _, err := c.scoreResult(ctx, query, result, rubric)
	return ans, err
}

func (c *Client) scoreResult(ctx context.Context, query string, result SearchResult, rubric []string) (*ScoreAnswer, Usage, error) {
	p, err := prompts.Load(prompts.RelevanceScore)
	if err != nil {
		return nil, Usage{}, err
	}
	if len(rubric) == 0 {
		rubric = p.Criteria
	}
	instructions, err := p.Render(prompts.Data{Query: query, Title: result.Title, URL: result.URL, Snippet: result.Snippet})
	if err != nil {
		return nil, Usage{}, err
	}
	resp, err := c.SystemOne(ctx, &SystemOneRequest{
		State:     resultState{Query: query, Result: result},
		Questions: map[string]Question{"relevance": ScoreQuestion(instructions, rubric)},
	})
	if err != nil {
		return nil, Usage{}, err
	}
	raw, ok := resp.Answers["relevance"]
	if !ok {
		return nil, resp.Usage, errors.New("jev: response missing \"relevance\" answer")
	}
	ans, err := raw.AsScore()
	if err != nil {
		return nil, resp.Usage, fmt.Errorf("jev: %w", err)
	}
	return ans, resp.Usage, nil
}

// ScoreBatch sends multiple results in one request. The returned map is keyed
// by BatchKey(i). Results that Jev did not answer are absent from the map.
func (c *Client) ScoreBatch(ctx context.Context, query string, results []SearchResult, rubric []string) (map[string]*ScoreAnswer, error) {
	out, _, err := c.scoreBatch(ctx, query, results, rubric)
	return out, err
}

func (c *Client) scoreBatch(ctx context.Context, query string, results []SearchResult, rubric []string) (map[string]*ScoreAnswer, Usage, error) {
	if len(results) == 0 {
		return map[string]*ScoreAnswer{}, Usage{}, nil
	}
	p, err := prompts.Load(prompts.RelevanceBatch)
	if err != nil {
		return nil, Usage{}, err
	}
	if len(rubric) == 0 {
		rubric = p.Criteria
	}
	state := batchState{Query: query, Results: make([]batchItem, 0, len(results))}
	questions := make(map[string]Question, len(results))
	for i, r := range results {
		id := BatchKey(i)
		state.Results = append(state.Results, batchItem{ID: id, SearchResult: r})
		instructions, err := p.Render(prompts.Data{Query: query, Title: r.Title, URL: r.URL, Snippet: r.Snippet, ID: id, Index: i})
		if err != nil {
			return nil, Usage{}, err
		}
		questions[id] = ScoreQuestion(instructions, rubric)
	}
	resp, err := c.SystemOne(ctx, &SystemOneRequest{State: state, Questions: questions})
	if err != nil {
		return nil, Usage{}, err
	}
	out := make(map[string]*ScoreAnswer, len(results))
	for key, raw := range resp.Answers {
		if ans, err := raw.AsScore(); err == nil {
			out[key] = ans
		}
	}
	return out, resp.Usage, nil
}

// NoulResult asks a yes/no question about a single result.
func (c *Client) NoulResult(ctx context.Context, query, question string, result SearchResult) (*NoulAnswer, error) {
	ans, _, err := c.noulResult(ctx, query, question, result)
	return ans, err
}

func (c *Client) noulResult(ctx context.Context, query, question string, result SearchResult) (*NoulAnswer, Usage, error) {
	if question == "" {
		return nil, Usage{}, errors.New("jev: noul question is empty")
	}
	p, err := prompts.Load(prompts.RelevanceNoul)
	if err != nil {
		return nil, Usage{}, err
	}
	instructions, err := p.Render(prompts.Data{Query: query, Question: question, Title: result.Title, URL: result.URL, Snippet: result.Snippet})
	if err != nil {
		return nil, Usage{}, err
	}
	resp, err := c.SystemOne(ctx, &SystemOneRequest{
		State:     resultState{Query: query, Result: result},
		Questions: map[string]Question{"answer": NoulQuestion(instructions)},
	})
	if err != nil {
		return nil, Usage{}, err
	}
	raw, ok := resp.Answers["answer"]
	if !ok {
		return nil, resp.Usage, errors.New("jev: response missing \"answer\"")
	}
	ans, err := raw.AsNoul()
	if err != nil {
		return nil, resp.Usage, fmt.Errorf("jev: %w", err)
	}
	return ans, resp.Usage, nil
}

// NoulBatch asks the same yes/no question about every result in one request.
// The returned map is keyed by BatchKey(i).
func (c *Client) NoulBatch(ctx context.Context, query, question string, results []SearchResult) (map[string]*NoulAnswer, error) {
	out, _, err := c.noulBatch(ctx, query, question, results)
	return out, err
}

func (c *Client) noulBatch(ctx context.Context, query, question string, results []SearchResult) (map[string]*NoulAnswer, Usage, error) {
	if question == "" {
		return nil, Usage{}, errors.New("jev: noul question is empty")
	}
	if len(results) == 0 {
		return map[string]*NoulAnswer{}, Usage{}, nil
	}
	p, err := prompts.Load(prompts.RelevanceNoul)
	if err != nil {
		return nil, Usage{}, err
	}
	state := batchState{Query: query, Results: make([]batchItem, 0, len(results))}
	questions := make(map[string]Question, len(results))
	for i, r := range results {
		id := BatchKey(i)
		state.Results = append(state.Results, batchItem{ID: id, SearchResult: r})
		instructions, err := p.Render(prompts.Data{Query: query, Question: question, Title: r.Title, URL: r.URL, Snippet: r.Snippet, ID: id, Index: i})
		if err != nil {
			return nil, Usage{}, err
		}
		questions[id] = NoulQuestion(fmt.Sprintf("(Result %s) %s", id, instructions))
	}
	resp, err := c.SystemOne(ctx, &SystemOneRequest{State: state, Questions: questions})
	if err != nil {
		return nil, Usage{}, err
	}
	out := make(map[string]*NoulAnswer, len(results))
	for key, raw := range resp.Answers {
		if ans, err := raw.AsNoul(); err == nil {
			out[key] = ans
		}
	}
	return out, resp.Usage, nil
}

// QualifyOptions configure Qualify.
type QualifyOptions struct {
	// Rubric overrides the default score criteria. Ignored when Noul is set.
	Rubric []string
	// Noul, when non-empty, asks this yes/no question instead of scoring.
	Noul string
	// Batch sends all results in a single request.
	Batch bool
	// Concurrency bounds parallel requests in per-result mode (default 8).
	Concurrency int
}

// Qualified pairs a search result with Jev's judgment of it.
type Qualified struct {
	Result provider.SearchResult `json:"result"`
	Score  *ScoreAnswer          `json:"score,omitempty"`
	Noul   *NoulAnswer           `json:"noul,omitempty"`
	// Err is set when Jev failed for this result (per-result mode) or returned
	// no answer for it (batch mode).
	Err error `json:"-"`
}

// Value returns the comparable relevance value: the score for score mode, or
// P(yes) for noul mode. Results with no answer yield -1 so they sort last.
func (q Qualified) Value() float64 {
	switch {
	case q.Score != nil:
		return q.Score.Score
	case q.Noul != nil:
		return q.Noul.Probability
	}
	return -1
}

// Max returns the top of the scale: len(rubric)-1 for scores, 1 for noul.
func (q Qualified) Max() float64 {
	switch {
	case q.Score != nil:
		return float64(q.Score.MaxScore())
	case q.Noul != nil:
		return 1
	}
	return 0
}

// Confidence returns Jev's confidence in the judgment.
func (q Qualified) Confidence() float64 {
	switch {
	case q.Score != nil:
		return q.Score.Confidence
	case q.Noul != nil:
		return q.Noul.Confidence()
	}
	return 0
}

// Qualify runs Jev over every result, honoring opts. The returned slice is
// aligned with results (same order, same length). Per-result failures are
// recorded in Qualified.Err rather than aborting the whole run; the returned
// error is non-nil only if every result failed or a batch request failed.
func (c *Client) Qualify(ctx context.Context, query string, results []SearchResult, opts QualifyOptions) ([]Qualified, Usage, error) {
	out := make([]Qualified, len(results))
	for i, r := range results {
		out[i].Result = r
	}
	if len(results) == 0 {
		return out, Usage{}, nil
	}

	if opts.Batch {
		return c.qualifyBatch(ctx, query, out, opts)
	}

	conc := opts.Concurrency
	if conc <= 0 {
		conc = DefaultConcurrency
	}
	if conc > len(results) {
		conc = len(results)
	}

	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		usage   Usage
		sem     = make(chan struct{}, conc)
		failed  int
		lastErr error
	)
	for i := range out {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int) {
			defer wg.Done()
			defer func() { <-sem }()
			var u Usage
			var err error
			if opts.Noul != "" {
				out[i].Noul, u, err = c.noulResult(ctx, query, opts.Noul, out[i].Result)
			} else {
				out[i].Score, u, err = c.scoreResult(ctx, query, out[i].Result, opts.Rubric)
			}
			mu.Lock()
			defer mu.Unlock()
			usage.Add(u)
			if err != nil {
				out[i].Err = err
				failed++
				lastErr = err
			}
		}(i)
	}
	wg.Wait()

	if failed == len(out) {
		return out, usage, fmt.Errorf("jev qualification failed for all %d results: %w", failed, lastErr)
	}
	return out, usage, nil
}

func (c *Client) qualifyBatch(ctx context.Context, query string, out []Qualified, opts QualifyOptions) ([]Qualified, Usage, error) {
	results := make([]SearchResult, len(out))
	for i := range out {
		results[i] = out[i].Result
	}
	if opts.Noul != "" {
		answers, usage, err := c.noulBatch(ctx, query, opts.Noul, results)
		if err != nil {
			return out, usage, err
		}
		for i := range out {
			if a, ok := answers[BatchKey(i)]; ok {
				out[i].Noul = a
			} else {
				out[i].Err = errors.New("jev: no answer returned for this result")
			}
		}
		return out, usage, nil
	}
	answers, usage, err := c.scoreBatch(ctx, query, results, opts.Rubric)
	if err != nil {
		return out, usage, err
	}
	for i := range out {
		if a, ok := answers[BatchKey(i)]; ok {
			out[i].Score = a
		} else {
			out[i].Err = errors.New("jev: no answer returned for this result")
		}
	}
	return out, usage, nil
}
