package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/dorkitude/multi_search_web/internal/config"
)

func runDocs(t *testing.T, args ...string) (string, error) {
	t.Helper()
	v = config.New()
	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(append([]string{"docs"}, args...))
	err := root.Execute()
	return out.String(), err
}

func TestDocsCommand(t *testing.T) {
	out, err := runDocs(t)
	if err != nil || !strings.Contains(out, "cooldowns") || !strings.Contains(out, "providers") {
		t.Errorf("list = %q, %v", out, err)
	}
	out, err = runDocs(t, "cooldowns")
	if err != nil || !strings.HasPrefix(out, "# Cooldowns") || !strings.Contains(out, "cooldown.steps") {
		t.Errorf("topic = %q, %v", out[:min(len(out), 80)], err)
	}
	out, err = runDocs(t, "all")
	if err != nil || strings.Count(out, "\n---\n") < 5 || !strings.Contains(out, "# Configuration") {
		t.Errorf("all: %d separators, %v", strings.Count(out, "\n---\n"), err)
	}
	if _, err := runDocs(t, "nope"); err == nil || !strings.Contains(err.Error(), "topics:") {
		t.Errorf("unknown topic err = %v", err)
	}
	// Every flag documented in search.md exists on the root command.
	body, _ := runDocs(t, "search")
	root := newRootCmd()
	for _, line := range strings.Split(body, "\n") {
		if !strings.HasPrefix(line, "| `-") {
			continue
		}
		cell := strings.TrimPrefix(strings.SplitN(line, "|", 3)[1], " ")
		cell = strings.Trim(strings.TrimSpace(cell), "`")
		name := cell
		if i := strings.Index(cell, ", --"); i >= 0 {
			name = cell[i+2:]
		}
		name = strings.TrimPrefix(strings.Fields(name)[0], "--")
		if root.Flags().Lookup(name) == nil && root.PersistentFlags().Lookup(name) == nil {
			t.Errorf("search.md documents unknown flag --%s", name)
		}
	}
}
