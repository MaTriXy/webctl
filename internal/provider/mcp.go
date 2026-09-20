package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// Some providers expose a keyless, hosted MCP (Model Context Protocol) server
// alongside their keyed REST API. A search is one JSON-RPC "tools/call"
// request; the answer is either a JSON body or a single-frame SSE stream
// whose last "data:" line carries the same JSON.

type mcpResponse struct {
	Result struct {
		Meta    map[string]any `json:"_meta"`
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		IsError bool `json:"isError"`
	} `json:"result"`
	Error *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// callMCPTool invokes tool with args on an MCP endpoint and returns the text
// blocks of the result. Transport failures never include the URL, since a key
// may be carried in its query string.
func callMCPTool(ctx context.Context, client *http.Client, providerName, endpoint, tool string, args map[string]any) ([]string, error) {
	return callMCPToolWithHeaders(ctx, client, providerName, endpoint, tool, args, nil)
}

// callMCPToolWithHeaders is callMCPTool with extra request headers.
func callMCPToolWithHeaders(ctx context.Context, client *http.Client, providerName, endpoint, tool string, args map[string]any, headers map[string]string) ([]string, error) {
	payload, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/call",
		"params":  map[string]any{"name": tool, "arguments": args},
	})
	if err != nil {
		return nil, fmt.Errorf("%s: encode request: %w", providerName, err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("%s: build request: %w", providerName, err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("User-Agent", "smart_search/1.0 (+https://github.com/dorkitude/smart_search)")
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s: request failed: %w", providerName, safeTransportError(err))
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, fmt.Errorf("%s: read response: %w", providerName, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &APIError{Provider: providerName, Status: resp.StatusCode, Body: summarizeBody(raw)}
	}
	body := raw
	if strings.HasPrefix(resp.Header.Get("Content-Type"), "text/event-stream") {
		if body, err = lastSSEData(raw); err != nil {
			return nil, fmt.Errorf("%s: %w", providerName, err)
		}
	}
	var rpc mcpResponse
	if err := json.Unmarshal(body, &rpc); err != nil {
		return nil, fmt.Errorf("%s: decode response: %w", providerName, err)
	}
	if rpc.Error != nil {
		return nil, fmt.Errorf("%s: JSON-RPC error %d: %s", providerName, rpc.Error.Code, rpc.Error.Message)
	}
	var texts []string
	for _, c := range rpc.Result.Content {
		if c.Type == "text" && strings.TrimSpace(c.Text) != "" {
			texts = append(texts, c.Text)
		}
	}
	// Free tiers report exhaustion inside a successful envelope: Exa sets
	// _meta["ai.exa/rateLimited"] and returns a notice as the only text.
	if rateLimited(rpc.Result.Meta, texts) {
		detail := ""
		if len(texts) > 0 {
			detail = truncate(collapseWhitespace(texts[0]), 160)
		}
		return nil, &APIError{Provider: providerName, Status: http.StatusTooManyRequests, Body: detail}
	}
	if rpc.Result.IsError {
		detail := "tool error"
		if len(texts) > 0 {
			detail = truncate(collapseWhitespace(texts[0]), 200)
		}
		return nil, fmt.Errorf("%s: %s", providerName, detail)
	}
	if len(texts) == 0 {
		return nil, fmt.Errorf("%s: response contained no text content", providerName)
	}
	return texts, nil
}

// rateLimited reports a keyless quota exhaustion hidden in a 200 response.
func rateLimited(meta map[string]any, texts []string) bool {
	for k, v := range meta {
		if strings.Contains(strings.ToLower(k), "ratelimited") {
			if b, ok := v.(bool); ok && b {
				return true
			}
		}
	}
	if len(texts) == 1 && len(texts[0]) < 600 {
		lower := strings.ToLower(texts[0])
		return strings.Contains(lower, "rate limit") || strings.Contains(lower, "quota")
	}
	return false
}

// lastSSEData returns the payload of the last non-empty "data:" line.
func lastSSEData(raw []byte) ([]byte, error) {
	var last []byte
	for _, line := range bytes.Split(raw, []byte("\n")) {
		line = bytes.TrimSpace(line)
		if data, ok := bytes.CutPrefix(line, []byte("data:")); ok {
			if data = bytes.TrimSpace(data); len(data) > 0 {
				last = data
			}
		}
	}
	if last == nil {
		return nil, errors.New("event stream contained no data payload")
	}
	return last, nil
}

// safeTransportError keeps the error's category (timeout, cancellation) but
// drops net/http's message, which embeds the request URL.
func safeTransportError(err error) error {
	switch {
	case errors.Is(err, context.Canceled):
		return context.Canceled
	case errors.Is(err, context.DeadlineExceeded):
		return context.DeadlineExceeded
	}
	if strings.Contains(err.Error(), "Client.Timeout") {
		return errors.New("timed out")
	}
	return errors.New("transport error")
}
