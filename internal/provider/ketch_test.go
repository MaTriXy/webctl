package provider

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestKetchSearchAndErrors(t *testing.T) {
	k := NewKetch("", Options{})
	var gotArgs []string
	k.run = func(_ context.Context, name string, args ...string) ([]byte, error) {
		gotArgs = append([]string{name}, args...)
		return []byte(`[{"title":" A ","url":"https://a.example","description":"Menu\nLog in\nA substantive description sentence that is long enough to count as prose here.","content":"body text"},{"title":"","url":"https://skip"}]`), nil
	}
	got, err := k.Search(context.Background(), "q", 7)
	if err != nil || len(got) != 1 || got[0].Title != "A" || got[0].Content != "body text" || !strings.HasPrefix(got[0].Snippet, "A substantive") {
		t.Errorf("got %+v, %v", got, err)
	}
	if strings.Join(gotArgs, " ") != "ketch search --json --limit 7 q" {
		t.Errorf("args = %v", gotArgs)
	}
	k = NewKetch("exa", Options{BaseURL: "/opt/ketch"})
	k.run = func(_ context.Context, name string, args ...string) ([]byte, error) {
		gotArgs = append([]string{name}, args...)
		return []byte(`[]`), nil
	}
	if _, err := k.Search(context.Background(), "q", 3); err != nil || strings.Join(gotArgs, " ") != "/opt/ketch search --json --limit 3 -b exa q" {
		t.Errorf("backend args = %v, %v", gotArgs, err)
	}

	k.run = func(context.Context, string, ...string) ([]byte, error) {
		return nil, ketchError(5, "Error: search failed: ddg rate limited after retries")
	}
	_, err = k.Search(context.Background(), "q", 3)
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Status != 429 {
		t.Errorf("rate limit should map to 429: %v", err)
	}
	k.run = func(context.Context, string, ...string) ([]byte, error) { return []byte("not json"), nil }
	if _, err := k.Search(context.Background(), "q", 3); err == nil || !strings.Contains(err.Error(), "decode") {
		t.Errorf("bad json err = %v", err)
	}
}
