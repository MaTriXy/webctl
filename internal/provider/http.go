package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

// APIError is returned when a provider responds with a non-2xx status.
type APIError struct {
	Provider string
	Status   int
	Body     string
}

func (e *APIError) Error() string {
	msg := fmt.Sprintf("%s API returned HTTP %d", e.Provider, e.Status)
	if e.Body != "" {
		msg += ": " + e.Body
	}
	switch e.Status {
	case http.StatusUnauthorized, http.StatusForbidden:
		msg += " (check your API key with `smart_search keys validate`)"
	case http.StatusTooManyRequests:
		msg += " (rate limited; try again shortly)"
	case http.StatusPaymentRequired:
		msg += " (free quota spent; add a key or wait)"
	}
	return msg
}

// Unauthorized reports whether the error indicates a rejected API key.
func (e *APIError) Unauthorized() bool {
	return e.Status == http.StatusUnauthorized || e.Status == http.StatusForbidden
}

// IsUnauthorized reports whether err wraps an APIError with a 401/403 status.
func IsUnauthorized(err error) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr) && apiErr.Unauthorized()
}

// Connection limits: a host that does not answer the TCP or TLS handshake
// is given up on quickly, so a dead provider costs seconds, not the whole
// request timeout.
const (
	dialTimeout         = 3 * time.Second
	tlsHandshakeTimeout = 5 * time.Second
)

// newHTTPClient returns opts.HTTPClient or a fresh client with the given timeout.
func newHTTPClient(opts Options, timeout time.Duration) *http.Client {
	if opts.HTTPClient != nil {
		return opts.HTTPClient
	}
	return &http.Client{Timeout: timeout, Transport: newTransport()}
}

// newTransport is http.DefaultTransport with short connect timeouts.
func newTransport() *http.Transport {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.DialContext = (&net.Dialer{Timeout: dialTimeout, KeepAlive: 30 * time.Second}).DialContext
	t.TLSHandshakeTimeout = tlsHandshakeTimeout
	return t
}

// postJSON marshals body, POSTs it, and decodes a 2xx JSON response into out.
// Non-2xx responses become *APIError with a truncated body for diagnostics.
func postJSON(ctx context.Context, client *http.Client, providerName, url string, headers map[string]string, body, out any) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("%s: encode request: %w", providerName, err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("%s: build request: %w", providerName, err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "smart_search/1.0 (+https://github.com/dorkitude/smart_search)")
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := client.Do(req)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return fmt.Errorf("%s: request timed out: %w", providerName, err)
		}
		return fmt.Errorf("%s: request failed: %w", providerName, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return &APIError{Provider: providerName, Status: resp.StatusCode, Body: summarizeBody(snippet)}
	}
	if out == nil {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("%s: decode response: %w", providerName, err)
	}
	return nil
}

// summarizeBody extracts a short, single-line message from an error body,
// preferring common {"error": ...} / {"message": ...} JSON shapes.
func summarizeBody(b []byte) string {
	s := strings.TrimSpace(string(b))
	if s == "" {
		return ""
	}
	var generic map[string]any
	if json.Unmarshal(b, &generic) == nil {
		for _, k := range []string{"error", "message", "detail", "msg"} {
			switch v := generic[k].(type) {
			case string:
				return truncate(collapseWhitespace(v), 200)
			case map[string]any:
				if m, ok := v["message"].(string); ok {
					return truncate(collapseWhitespace(m), 200)
				}
			}
		}
	}
	return truncate(collapseWhitespace(s), 200)
}
