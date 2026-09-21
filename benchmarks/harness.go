package benchmarks

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Mode is how an arm reaches the web.
type Mode string

const (
	// ModeWebctl: the agent may only use `webctl` from its shell.
	ModeWebctl Mode = "webctl"
	// ModeWebctlLite: webctl only, and never --scrape: results are title,
	// URL, score, and snippet, the same shape as a native search result.
	ModeWebctlLite Mode = "webctl-lite"
	// ModeNative: the agent may only use its own built-in search and fetch.
	ModeNative Mode = "native"
)

// UsesWebctl reports whether the mode reaches the web through webctl.
func (m Mode) UsesWebctl() bool { return m == ModeWebctl || m == ModeWebctlLite }

// Arm is one harness × model × mode combination.
type Arm struct {
	Name    string `json:"name"`    // e.g. claude-sonnet-webctl
	Harness string `json:"harness"` // claude, codex, pi
	Model   string `json:"model"`
	Mode    Mode   `json:"mode"`
}

// Tokens is a harness-independent token count. Total input is
// Input+CacheRead+CacheWrite: every token the model attended to, cached or
// not, since caching changes price but not context usage.
type Tokens struct {
	Input      int `json:"input"`       // uncached prompt tokens
	CacheRead  int `json:"cache_read"`  // prompt tokens served from cache
	CacheWrite int `json:"cache_write"` // prompt tokens written to cache
	Output     int `json:"output"`      // includes reasoning where reported
	Reasoning  int `json:"reasoning,omitempty"`
}

// TotalInput is every prompt token processed across all turns.
func (t Tokens) TotalInput() int { return t.Input + t.CacheRead + t.CacheWrite }

// Total is all tokens in and out.
func (t Tokens) Total() int { return t.TotalInput() + t.Output }

func (t *Tokens) add(o Tokens) {
	t.Input += o.Input
	t.CacheRead += o.CacheRead
	t.CacheWrite += o.CacheWrite
	t.Output += o.Output
	t.Reasoning += o.Reasoning
}

// Result is one arm's attempt at one case.
type Result struct {
	Case   string `json:"case"`
	Arm    string `json:"arm"`
	Answer string `json:"answer"`
	Error  string `json:"error,omitempty"`
	WallMs int64  `json:"wall_ms"`
	Tokens Tokens `json:"tokens"`
	// CostUSD is what the harness reported; 0 when it reports none (codex).
	CostUSD float64 `json:"cost_usd"`
	// Turns counts model calls (assistant messages) where the harness says.
	Turns int `json:"turns"`
	// WebctlCalls and SearchCalls count tool use: shell commands running
	// webctl, and native web search/fetch calls.
	WebctlCalls int `json:"webctl_calls"`
	SearchCalls int `json:"search_calls"`
	// Violations counts tool calls the arm was told not to make (a native
	// search in a webctl arm, a shell command in a native arm).
	Violations int `json:"violations"`
	// PayloadChars is the size of every search-tool result returned into
	// the agent's context: webctl output, or the harness's own search and
	// fetch results. 0 when the harness hides them (Codex native search).
	PayloadChars int `json:"payload_chars"`
	// Log is the path of the raw harness output.
	Log string `json:"log,omitempty"`
	// Judge is filled after grading.
	Judge *Grade `json:"judge,omitempty"`
}

// Runner executes an arm on a prompt. workDir is an empty directory the
// agent runs in; logDir receives raw output.
type Runner interface {
	Run(ctx context.Context, arm Arm, prompt string, workDir, logDir string) (Result, error)
}

// NewRunner returns the runner for a harness name.
func NewRunner(harness string) (Runner, error) {
	switch harness {
	case "claude":
		return claudeRunner{}, nil
	case "codex":
		return codexRunner{}, nil
	case "pi":
		return piRunner{}, nil
	}
	return nil, fmt.Errorf("unknown harness %q (claude, codex, pi)", harness)
}

// DefaultArms is the matrix the benchmark runs unless told otherwise.
// pi has no built-in web search, so it has only webctl arms. Codex arms
// exist (AllArms) but are not default: its native search is server-side,
// so what it puts into context is not observable.
func DefaultArms() []Arm {
	var out []Arm
	for _, a := range AllArms() {
		if a.Harness != "codex" {
			out = append(out, a)
		}
	}
	return out
}

// AllArms is every arm the tool knows, including Codex.
func AllArms() []Arm {
	return []Arm{
		{Name: "claude-sonnet-webctl", Harness: "claude", Model: "sonnet", Mode: ModeWebctl},
		{Name: "claude-sonnet-webctl-lite", Harness: "claude", Model: "sonnet", Mode: ModeWebctlLite},
		{Name: "claude-sonnet-native", Harness: "claude", Model: "sonnet", Mode: ModeNative},
		{Name: "codex-terra-webctl", Harness: "codex", Model: "gpt-5.6-terra", Mode: ModeWebctl},
		{Name: "codex-terra-webctl-lite", Harness: "codex", Model: "gpt-5.6-terra", Mode: ModeWebctlLite},
		{Name: "codex-terra-native", Harness: "codex", Model: "gpt-5.6-terra", Mode: ModeNative},
		{Name: "pi-kimi-k3-webctl", Harness: "pi", Model: "accounts/fireworks/models/kimi-k3", Mode: ModeWebctl},
		{Name: "pi-kimi-k3-webctl-lite", Harness: "pi", Model: "accounts/fireworks/models/kimi-k3", Mode: ModeWebctlLite},
	}
}

// ParseArms turns "claude-sonnet-webctl,codex-terra-native" into arms from
// AllArms, or errors on an unknown name. "" and "default" give DefaultArms;
// "all" gives every arm including Codex.
func ParseArms(spec string) ([]Arm, error) {
	all := AllArms()
	switch strings.TrimSpace(spec) {
	case "", "default":
		return DefaultArms(), nil
	case "all":
		return all, nil
	}
	var out []Arm
	for _, name := range strings.Split(spec, ",") {
		name = strings.TrimSpace(name)
		found := false
		for _, a := range all {
			if a.Name == name {
				out = append(out, a)
				found = true
			}
		}
		if !found {
			return nil, fmt.Errorf("unknown arm %q", name)
		}
	}
	return out, nil
}

// run executes a command with stdin closed, capturing stdout and stderr,
// and writes both to logDir. It returns stdout.
func run(ctx context.Context, logDir, logName string, env []string, name string, args ...string) ([]byte, []byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdin = nil
	cmd.Env = env
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	if logDir != "" {
		_ = os.WriteFile(filepath.Join(logDir, logName+".stdout"), stdout.Bytes(), 0o644)
		if stderr.Len() > 0 {
			_ = os.WriteFile(filepath.Join(logDir, logName+".stderr"), stderr.Bytes(), 0o644)
		}
	}
	return stdout.Bytes(), stderr.Bytes(), err
}

// hostEnv is the environment for a harness: the user's PATH and home so
// `webctl`, keys, and auth resolve, minus the variables that make Claude
// Code refuse to run inside another Claude Code session.
func hostEnv() []string {
	var env []string
	for _, kv := range os.Environ() {
		if strings.HasPrefix(kv, "CLAUDECODE=") || strings.HasPrefix(kv, "CLAUDE_CODE_ENTRYPOINT=") {
			continue
		}
		env = append(env, kv)
	}
	return env
}

// ---- Claude Code ----

type claudeRunner struct{}

func (claudeRunner) Run(ctx context.Context, arm Arm, prompt, workDir, logDir string) (Result, error) {
	args := []string{
		"-p", prompt,
		"--model", arm.Model,
		"--output-format", "stream-json", "--verbose",
		"--no-session-persistence",
		"--max-turns", "40",
	}
	switch arm.Mode {
	case ModeWebctl, ModeWebctlLite:
		args = append(args,
			"--allowedTools", "Bash(webctl:*)",
			"--disallowedTools", "WebSearch,WebFetch,Agent,Read,Edit,Write,Glob,Grep",
		)
	case ModeNative:
		args = append(args,
			"--allowedTools", "WebSearch,WebFetch",
			"--disallowedTools", "Bash,Agent,Read,Edit,Write,Glob,Grep",
		)
	}
	cmd := exec.CommandContext(ctx, "claude", args...)
	cmd.Dir = workDir
	cmd.Env = hostEnv()
	cmd.Stdin = nil
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	start := time.Now()
	runErr := cmd.Run()
	wall := time.Since(start)
	logPath := filepath.Join(logDir, arm.Name+".jsonl")
	_ = os.WriteFile(logPath, stdout.Bytes(), 0o644)
	if stderr.Len() > 0 {
		_ = os.WriteFile(filepath.Join(logDir, arm.Name+".stderr"), stderr.Bytes(), 0o644)
	}
	res := Result{Arm: arm.Name, WallMs: wall.Milliseconds(), Log: logPath}
	parsed, err := parseClaude(stdout.Bytes())
	if err != nil {
		if runErr != nil {
			return res, fmt.Errorf("claude: %v: %s", runErr, firstLine(stderr.String()))
		}
		return res, fmt.Errorf("claude: %w", err)
	}
	res.Answer, res.Tokens, res.CostUSD, res.Turns = parsed.answer, parsed.tokens, parsed.cost, parsed.turns
	res.SearchCalls = parsed.searches
	res.WebctlCalls = parsed.bashCalls // only webctl is allowed in the shell
	if parsed.isError {
		res.Error = "claude reported is_error"
	}
	switch arm.Mode {
	case ModeWebctl, ModeWebctlLite:
		res.Violations = parsed.searches
	case ModeNative:
		res.Violations = parsed.bashCalls
		res.WebctlCalls = 0
	}
	res.PayloadChars = parsed.payload
	return res, nil
}

type claudeParsed struct {
	answer    string
	tokens    Tokens
	cost      float64
	turns     int
	searches  int
	bashCalls int
	isError   bool
	payload   int
}

// parseClaude reads the stream-json events: tool_use blocks in assistant
// messages give exact tool counts, and the final result event gives usage.
func parseClaude(b []byte) (claudeParsed, error) {
	var p claudeParsed
	sc := bufio.NewScanner(bytes.NewReader(b))
	sc.Buffer(make([]byte, 1<<20), 64<<20)
	gotResult := false
	tools := map[string]string{} // tool_use id → tool name
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 || line[0] != '{' {
			continue
		}
		var ev struct {
			Type    string `json:"type"`
			Message struct {
				Content []struct {
					Type      string          `json:"type"`
					Name      string          `json:"name"`
					ID        string          `json:"id"`
					ToolUseID string          `json:"tool_use_id"`
					Content   json.RawMessage `json:"content"`
				} `json:"content"`
			} `json:"message"`
			Result   string  `json:"result"`
			IsError  bool    `json:"is_error"`
			NumTurns int     `json:"num_turns"`
			Cost     float64 `json:"total_cost_usd"`
			Usage    struct {
				Input      int `json:"input_tokens"`
				CacheRead  int `json:"cache_read_input_tokens"`
				CacheWrite int `json:"cache_creation_input_tokens"`
				Output     int `json:"output_tokens"`
				Details    struct {
					Thinking int `json:"thinking_tokens"`
				} `json:"output_tokens_details"`
			} `json:"usage"`
		}
		if err := json.Unmarshal(line, &ev); err != nil {
			continue
		}
		switch ev.Type {
		case "assistant":
			for _, c := range ev.Message.Content {
				if c.Type != "tool_use" {
					continue
				}
				tools[c.ID] = c.Name
				switch c.Name {
				case "WebSearch", "WebFetch":
					p.searches++
				case "Bash":
					p.bashCalls++
				}
			}
		case "user":
			for _, c := range ev.Message.Content {
				if c.Type != "tool_result" {
					continue
				}
				switch tools[c.ToolUseID] {
				case "WebSearch", "WebFetch", "Bash":
					p.payload += toolResultChars(c.Content)
				}
			}
		case "result":
			gotResult = true
			p.answer, p.cost, p.turns, p.isError = ev.Result, ev.Cost, ev.NumTurns, ev.IsError
			p.tokens = Tokens{
				Input:      ev.Usage.Input,
				CacheRead:  ev.Usage.CacheRead,
				CacheWrite: ev.Usage.CacheWrite,
				Output:     ev.Usage.Output,
				Reasoning:  ev.Usage.Details.Thinking,
			}
		}
	}
	if !gotResult {
		return p, fmt.Errorf("no result event in output (%s)", firstLine(string(b)))
	}
	return p, nil
}

// ---- Codex ----

type codexRunner struct{}

func (codexRunner) Run(ctx context.Context, arm Arm, prompt, workDir, logDir string) (Result, error) {
	args := []string{
		"exec", "--json", "--ephemeral", "--skip-git-repo-check", "--ignore-user-config",
		"-m", arm.Model,
		"-C", workDir,
	}
	switch arm.Mode {
	case ModeWebctl, ModeWebctlLite:
		args = append(args, "--dangerously-bypass-approvals-and-sandbox", "-c", `web_search="disabled"`)
	case ModeNative:
		args = append(args, "-s", "read-only", "-c", `web_search="live"`)
	}
	args = append(args, prompt)
	start := time.Now()
	stdout, stderr, runErr := run(ctx, logDir, arm.Name, hostEnv(), "codex", args...)
	wall := time.Since(start)
	res := Result{Arm: arm.Name, WallMs: wall.Milliseconds(), Log: filepath.Join(logDir, arm.Name+".stdout")}
	parsed, err := parseCodex(stdout)
	if err != nil {
		if runErr != nil {
			return res, fmt.Errorf("codex: %v: %s", runErr, firstLine(string(stderr)))
		}
		return res, fmt.Errorf("codex: %w", err)
	}
	res.Answer, res.Tokens, res.Turns = parsed.answer, parsed.tokens, parsed.turns
	res.SearchCalls, res.WebctlCalls = parsed.searches, parsed.webctl
	switch arm.Mode {
	case ModeWebctl, ModeWebctlLite:
		res.Violations = parsed.searches
	case ModeNative:
		res.Violations = parsed.commands
		res.WebctlCalls = 0
	}
	res.PayloadChars = parsed.payload // native search results are server-side: not visible
	if parsed.errText != "" {
		res.Error = parsed.errText
	}
	return res, nil
}

type codexParsed struct {
	answer   string
	tokens   Tokens
	turns    int
	searches int
	webctl   int
	commands int
	payload  int
	errText  string
}

func parseCodex(b []byte) (codexParsed, error) {
	var p codexParsed
	sc := bufio.NewScanner(bytes.NewReader(b))
	sc.Buffer(make([]byte, 1<<20), 64<<20)
	seen := false
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 || line[0] != '{' {
			continue
		}
		var ev struct {
			Type string `json:"type"`
			Item struct {
				Type    string `json:"type"`
				Text    string `json:"text"`
				Command string `json:"command"`
				Output  string `json:"aggregated_output"`
			} `json:"item"`
			Usage struct {
				Input      int `json:"input_tokens"`
				Cached     int `json:"cached_input_tokens"`
				CacheWrite int `json:"cache_write_input_tokens"`
				Output     int `json:"output_tokens"`
				Reasoning  int `json:"reasoning_output_tokens"`
			} `json:"usage"`
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
			Message string `json:"message"`
		}
		if err := json.Unmarshal(line, &ev); err != nil {
			continue
		}
		seen = true
		switch ev.Type {
		case "item.completed":
			switch ev.Item.Type {
			case "agent_message":
				p.answer = ev.Item.Text
				p.turns++
			case "web_search":
				p.searches++
			case "command_execution":
				p.commands++
				p.payload += len(ev.Item.Output)
				if strings.Contains(ev.Item.Command, "webctl") {
					p.webctl++
				}
			}
		case "turn.completed":
			// Codex reports input_tokens inclusive of cached tokens.
			p.tokens.add(Tokens{
				Input:      ev.Usage.Input - ev.Usage.Cached,
				CacheRead:  ev.Usage.Cached,
				CacheWrite: ev.Usage.CacheWrite,
				Output:     ev.Usage.Output,
				Reasoning:  ev.Usage.Reasoning,
			})
		case "error", "turn.failed":
			if ev.Error.Message != "" {
				p.errText = ev.Error.Message
			} else if ev.Message != "" {
				p.errText = ev.Message
			}
		}
	}
	if !seen {
		return p, errors.New("no JSONL events in output")
	}
	if p.answer == "" && p.errText == "" {
		return p, errors.New("no agent_message in output")
	}
	return p, nil
}

// ---- pi ----

type piRunner struct{}

// PiArgs are the flags that make pi headless and quiet. The judge reuses them.
func piArgs(model string) []string {
	provider, id := "fireworks", model
	if i := strings.Index(model, "/"); i > 0 && !strings.HasPrefix(model, "accounts/") {
		provider, id = model[:i], model[i+1:]
	}
	return []string{"-p", "--mode", "json", "--no-session", "--no-extensions", "--no-skills",
		"--provider", provider, "--model", id}
}

func (piRunner) Run(ctx context.Context, arm Arm, prompt, workDir, logDir string) (Result, error) {
	if !arm.Mode.UsesWebctl() {
		return Result{Arm: arm.Name}, errors.New("pi has no built-in web search; only the webctl modes exist")
	}
	promptArg, err := piPromptFile(filepath.Join(logDir, arm.Name+".prompt.md"), prompt)
	if err != nil {
		return Result{Arm: arm.Name}, err
	}
	args := append(piArgs(arm.Model), "--tools", "bash", promptArg)
	cmd := exec.CommandContext(ctx, "pi", args...)
	cmd.Dir = workDir
	cmd.Env = hostEnv()
	cmd.Stdin = nil
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	start := time.Now()
	runErr := cmd.Run()
	wall := time.Since(start)
	logPath := filepath.Join(logDir, arm.Name+".jsonl")
	_ = os.WriteFile(logPath, stdout.Bytes(), 0o644)
	if stderr.Len() > 0 {
		_ = os.WriteFile(filepath.Join(logDir, arm.Name+".stderr"), stderr.Bytes(), 0o644)
	}
	res := Result{Arm: arm.Name, WallMs: wall.Milliseconds(), Log: logPath}
	parsed, err := parsePi(stdout.Bytes())
	if err != nil {
		if runErr != nil {
			return res, fmt.Errorf("pi: %v: %s", runErr, firstLine(stderr.String()))
		}
		return res, fmt.Errorf("pi: %w", err)
	}
	res.Answer, res.Tokens, res.CostUSD, res.Turns = parsed.answer, parsed.tokens, parsed.cost, parsed.turns
	res.WebctlCalls = parsed.webctl
	res.Violations = parsed.commands - parsed.webctl
	res.PayloadChars = parsed.payload
	if parsed.errText != "" {
		res.Error = parsed.errText
	}
	return res, nil
}

type piParsed struct {
	answer   string
	tokens   Tokens
	cost     float64
	turns    int
	webctl   int
	commands int
	payload  int
	errText  string
}

func parsePi(b []byte) (piParsed, error) {
	var p piParsed
	sc := bufio.NewScanner(bytes.NewReader(b))
	sc.Buffer(make([]byte, 1<<20), 64<<20)
	seen := false
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 || line[0] != '{' {
			continue
		}
		var ev struct {
			Type    string `json:"type"`
			Message struct {
				Role    string `json:"role"`
				Content []struct {
					Type string `json:"type"`
					Text string `json:"text"`
				} `json:"content"`
				Usage struct {
					Input      int `json:"input"`
					Output     int `json:"output"`
					CacheRead  int `json:"cacheRead"`
					CacheWrite int `json:"cacheWrite"`
					Cost       struct {
						Total float64 `json:"total"`
					} `json:"cost"`
				} `json:"usage"`
				StopReason   string `json:"stopReason"`
				ErrorMessage string `json:"errorMessage"`
			} `json:"message"`
			ToolName string `json:"toolName"`
			Args     struct {
				Command string `json:"command"`
			} `json:"args"`
			Result struct {
				Content json.RawMessage `json:"content"`
			} `json:"result"`
			Error string `json:"error"`
		}
		if err := json.Unmarshal(line, &ev); err != nil {
			continue
		}
		seen = true
		switch ev.Type {
		case "message_end":
			if ev.Message.Role != "assistant" {
				continue
			}
			p.turns++
			p.tokens.add(Tokens{
				Input:      ev.Message.Usage.Input,
				CacheRead:  ev.Message.Usage.CacheRead,
				CacheWrite: ev.Message.Usage.CacheWrite,
				Output:     ev.Message.Usage.Output,
			})
			p.cost += ev.Message.Usage.Cost.Total
			var text strings.Builder
			for _, c := range ev.Message.Content {
				if c.Type == "text" {
					text.WriteString(c.Text)
				}
			}
			if s := strings.TrimSpace(text.String()); s != "" {
				p.answer = s
			}
			if ev.Message.StopReason == "error" && ev.Message.ErrorMessage != "" {
				p.errText = ev.Message.ErrorMessage
			}
		case "tool_execution_start":
			if ev.ToolName == "bash" {
				p.commands++
				if strings.Contains(ev.Args.Command, "webctl") {
					p.webctl++
				}
			}
		case "tool_execution_end":
			if ev.ToolName == "bash" {
				p.payload += toolResultChars(ev.Result.Content)
			}
		}
	}
	if !seen {
		return p, errors.New("no JSON events in output")
	}
	if p.answer == "" && p.errText == "" {
		return p, errors.New("no assistant text in output")
	}
	return p, nil
}

// toolResultChars sizes a tool result that is either a JSON string or a
// list of {type, text} blocks.
func toolResultChars(raw json.RawMessage) int {
	if len(raw) == 0 {
		return 0
	}
	var str string
	if json.Unmarshal(raw, &str) == nil {
		return len(str)
	}
	var blocks []struct {
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &blocks) == nil {
		n := 0
		for _, b := range blocks {
			n += len(b.Text)
		}
		return n
	}
	return len(raw)
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if len(s) > 300 {
		s = s[:300]
	}
	return s
}

// piPromptFile writes prompt to path and returns the "@path" argument pi
// reads it from. pi is passed its prompt this way rather than as an
// argument: on macOS, a pi invocation whose prompt argument is longer than
// about 1,000 bytes dies with SIGKILL at exec, before node starts (measured
// 2026-09-20 with pi 0.65.0; 928-byte prompts run, 1,011-byte ones die).
// pi includes the file's text in the user message inside a <file> tag.
func piPromptFile(path, prompt string) (string, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, []byte(prompt), 0o644); err != nil {
		return "", err
	}
	return "@" + path, nil
}
