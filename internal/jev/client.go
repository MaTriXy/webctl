package jev

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Defaults.
const (
	DefaultBaseURL = "https://api.typesafe.ai"
	DefaultModel   = "jev-latest"
	DefaultTimeout = 60 * time.Second
	systemOnePath  = "/v1/systemone"
	maxAttempts    = 3
)

// Client talks to the Jev System One API.
type Client struct {
	APIKey  string
	BaseURL string // default: https://api.typesafe.ai
	Model   string // default: jev-latest

	// HTTPClient defaults to one with DefaultTimeout.
	HTTPClient *http.Client
	// Retry controls whether 429/5xx responses are retried with backoff (default true).
	NoRetry bool
	// sleep is swapped in tests.
	sleep func(context.Context, time.Duration) error
}

// NewClient returns a Client with defaults filled in.
func NewClient(apiKey string) *Client {
	return &Client{APIKey: apiKey}
}

// APIError is a non-2xx response from Jev.
type APIError struct {
	Status int
	Body   string
}

func (e *APIError) Error() string {
	msg := fmt.Sprintf("Jev API returned HTTP %d", e.Status)
	if e.Body != "" {
		msg += ": " + e.Body
	}
	switch e.Status {
	case http.StatusUnauthorized, http.StatusForbidden:
		msg += " (check JEV_API_KEY or run `webctl setup`)"
	case http.StatusTooManyRequests:
		msg += " (rate limited)"
	}
	return msg
}

// Unauthorized reports whether the key was rejected.
func (e *APIError) Unauthorized() bool {
	return e.Status == http.StatusUnauthorized || e.Status == http.StatusForbidden
}

// IsUnauthorized reports whether err is a 401/403 from Jev.
func IsUnauthorized(err error) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr) && apiErr.Unauthorized()
}

func (c *Client) baseURL() string {
	if c.BaseURL == "" {
		return DefaultBaseURL
	}
	return strings.TrimRight(c.BaseURL, "/")
}

func (c *Client) model() string {
	if c.Model == "" {
		return DefaultModel
	}
	return c.Model
}

func (c *Client) httpClient() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}
	return &http.Client{Timeout: DefaultTimeout}
}

func (c *Client) doSleep(ctx context.Context, d time.Duration) error {
	if c.sleep != nil {
		return c.sleep(ctx, d)
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// SystemOne sends a request to POST /v1/systemone. If req.Model is empty the
// client's model is used. Transient failures (429, 5xx, network) are retried
// up to maxAttempts times with exponential backoff unless NoRetry is set.
func (c *Client) SystemOne(ctx context.Context, req *SystemOneRequest) (*SystemOneResponse, error) {
	if c.APIKey == "" {
		return nil, errors.New("jev: API key is empty")
	}
	if req == nil {
		return nil, errors.New("jev: nil request")
	}
	if len(req.Questions) == 0 {
		return nil, errors.New("jev: request has no questions")
	}
	if req.Model == "" {
		req.Model = c.model()
	}
	payload, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("jev: encode request: %w", err)
	}

	attempts := maxAttempts
	if c.NoRetry {
		attempts = 1
	}
	backoff := 500 * time.Millisecond
	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		resp, err := c.once(ctx, payload)
		if err == nil {
			return resp, nil
		}
		lastErr = err
		if !retryable(err) || attempt == attempts {
			break
		}
		if err := c.doSleep(ctx, backoff); err != nil {
			return nil, err
		}
		backoff *= 2
	}
	return nil, lastErr
}

func (c *Client) once(ctx context.Context, payload []byte) (*SystemOneResponse, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL()+systemOnePath, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("jev: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.APIKey)
	httpReq.Header.Set("User-Agent", "webctl/1.0 (+https://github.com/dorkitude/webctl)")

	resp, err := c.httpClient().Do(httpReq)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, fmt.Errorf("jev: request timed out: %w", err)
		}
		return nil, fmt.Errorf("jev: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, &APIError{Status: resp.StatusCode, Body: summarize(snippet)}
	}
	var out SystemOneResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("jev: decode response: %w", err)
	}
	return &out, nil
}

// retryable reports whether an error is worth retrying.
func retryable(err error) bool {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr.Status == http.StatusTooManyRequests || apiErr.Status >= 500
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	// Network-level errors (connection reset, EOF, ...) are transient.
	return true
}

// Validate confirms the API key works with a trivial noul question.
func (c *Client) Validate(ctx context.Context) error {
	req := &SystemOneRequest{
		State:     map[string]any{"text": "hello world"},
		Questions: map[string]Question{"greeting": NoulQuestion("Is the text a greeting?")},
	}
	_, err := c.SystemOne(ctx, req)
	return err
}

func summarize(b []byte) string {
	s := strings.TrimSpace(string(b))
	if s == "" {
		return ""
	}
	var m map[string]any
	if json.Unmarshal(b, &m) == nil {
		for _, k := range []string{"error", "message", "detail"} {
			switch v := m[k].(type) {
			case string:
				return clip(v)
			case map[string]any:
				if msg, ok := v["message"].(string); ok {
					return clip(msg)
				}
			}
		}
	}
	return clip(s)
}

func clip(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > 200 {
		return s[:200] + "…"
	}
	return s
}
