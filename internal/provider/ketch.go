package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// Ketch delegates to the ketch CLI (github.com/1broseidon/ketch), which
// runs its own chain of keyless and keyed backends. It needs no key here;
// ketch's own configuration decides what it uses.
type Ketch struct {
	// Binary is the ketch executable (default "ketch" on PATH).
	Binary string
	// Backend is passed as -b; "" lets ketch pick (its "auto" chain).
	Backend string
	// run is overridable by tests.
	run func(ctx context.Context, name string, args ...string) ([]byte, error)
}

// NewKetch constructs a Ketch provider. cred, if non-empty, selects the
// ketch backend (for example "exa" or "auto").
func NewKetch(cred string, opts Options) *Ketch {
	k := &Ketch{Binary: "ketch", Backend: strings.TrimSpace(cred)}
	if opts.BaseURL != "" {
		k.Binary = opts.BaseURL
	}
	k.run = func(ctx context.Context, name string, args ...string) ([]byte, error) {
		cmd := exec.CommandContext(ctx, name, args...)
		var stdout, stderr bytes.Buffer
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		if err := cmd.Run(); err != nil {
			var exitErr *exec.ExitError
			msg := strings.TrimSpace(stderr.String())
			if errors.As(err, &exitErr) {
				if msg == "" {
					msg = "exit " + strconv.Itoa(exitErr.ExitCode())
				}
				return nil, ketchError(exitErr.ExitCode(), msg)
			}
			if errors.Is(err, exec.ErrNotFound) {
				return nil, errors.New("ketch is not installed (https://github.com/1broseidon/ketch)")
			}
			return nil, err
		}
		return stdout.Bytes(), nil
	}
	return k
}

// ketchError maps ketch's exit codes onto APIError statuses so the chain's
// cooldown treats a throttled ketch like any other throttled provider.
func ketchError(code int, msg string) error {
	lower := strings.ToLower(msg)
	switch {
	case strings.Contains(lower, "rate limit") || strings.Contains(lower, "429"):
		return &APIError{Provider: "ketch", Status: 429, Body: truncate(collapseWhitespace(msg), 200)}
	case strings.Contains(lower, "quota") || strings.Contains(lower, "402"):
		return &APIError{Provider: "ketch", Status: 402, Body: truncate(collapseWhitespace(msg), 200)}
	}
	return fmt.Errorf("ketch: %s", truncate(collapseWhitespace(msg), 200))
}

// Name implements Provider.
func (k *Ketch) Name() string { return "ketch" }

type ketchResult struct {
	Title       string `json:"title"`
	URL         string `json:"url"`
	Description string `json:"description"`
	Content     string `json:"content"`
}

// Search implements Provider.
func (k *Ketch) Search(ctx context.Context, query string, numResults int) ([]SearchResult, error) {
	args := []string{"search", "--json", "--limit", strconv.Itoa(clampNum(numResults, 50))}
	if k.Backend != "" {
		args = append(args, "-b", k.Backend)
	}
	args = append(args, query)
	raw, err := k.run(ctx, k.Binary, args...)
	if err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("ketch: %w", ctx.Err())
		}
		return nil, err
	}
	var items []ketchResult
	if err := json.Unmarshal(bytes.TrimSpace(raw), &items); err != nil {
		return nil, fmt.Errorf("ketch: decode output: %w", err)
	}
	out := make([]SearchResult, 0, len(items))
	for _, it := range items {
		title, url := strings.TrimSpace(it.Title), strings.TrimSpace(it.URL)
		if title == "" || url == "" {
			continue
		}
		content := strings.TrimSpace(it.Content)
		snippet := strings.TrimSpace(it.Description + "\n" + content)
		out = append(out, SearchResult{Title: collapseWhitespace(title), URL: url, Snippet: excerpt(snippet), Content: content})
	}
	return out, nil
}

// Validate implements Provider with a one-result search.
func (k *Ketch) Validate(ctx context.Context) error {
	_, err := k.Search(ctx, "hello world", 1)
	return err
}
