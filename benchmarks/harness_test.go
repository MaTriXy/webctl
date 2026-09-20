package benchmarks

import (
	"strings"
	"testing"
)

func TestParseClaudeStream(t *testing.T) {
	stream := strings.Join([]string{
		`{"type":"system","subtype":"init"}`,
		`{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Bash","input":{"command":"webctl search q"}}]}}`,
		`{"type":"user","message":{"content":[{"type":"tool_result"}]}}`,
		`{"type":"assistant","message":{"content":[{"type":"tool_use","name":"WebSearch","input":{}}]}}`,
		`{"type":"assistant","message":{"content":[{"type":"text","text":"done"}]}}`,
		`{"type":"result","subtype":"success","result":"The answer.","is_error":false,"num_turns":3,"total_cost_usd":0.25,"usage":{"input_tokens":10,"cache_read_input_tokens":500,"cache_creation_input_tokens":300,"output_tokens":40,"output_tokens_details":{"thinking_tokens":5}}}`,
	}, "\n")
	p, err := parseClaude([]byte(stream))
	if err != nil {
		t.Fatal(err)
	}
	if p.answer != "The answer." || p.turns != 3 || p.cost != 0.25 || p.bashCalls != 1 || p.searches != 1 {
		t.Errorf("parsed = %+v", p)
	}
	if p.tokens.TotalInput() != 810 || p.tokens.Output != 40 || p.tokens.Reasoning != 5 {
		t.Errorf("tokens = %+v", p.tokens)
	}
	if _, err := parseClaude([]byte(`{"type":"system"}`)); err == nil {
		t.Error("missing result should error")
	}
}

func TestParseCodexJSONL(t *testing.T) {
	stream := strings.Join([]string{
		`Reading additional input from stdin...`,
		`{"type":"thread.started","thread_id":"x"}`,
		`{"type":"item.completed","item":{"id":"1","type":"agent_message","text":"working"}}`,
		`{"type":"item.completed","item":{"id":"2","type":"web_search","query":"q"}}`,
		`{"type":"item.completed","item":{"id":"3","type":"command_execution","command":"/bin/zsh -lc 'webctl search q'"}}`,
		`{"type":"item.completed","item":{"id":"4","type":"command_execution","command":"ls"}}`,
		`{"type":"item.completed","item":{"id":"5","type":"agent_message","text":"Final answer"}}`,
		`{"type":"turn.completed","usage":{"input_tokens":12947,"cached_input_tokens":9984,"cache_write_input_tokens":0,"output_tokens":5,"reasoning_output_tokens":2}}`,
	}, "\n")
	p, err := parseCodex([]byte(stream))
	if err != nil {
		t.Fatal(err)
	}
	if p.answer != "Final answer" || p.turns != 2 || p.searches != 1 || p.webctl != 1 || p.commands != 2 {
		t.Errorf("parsed = %+v", p)
	}
	// input_tokens includes cached tokens; they must not be double counted.
	if p.tokens.Input != 2963 || p.tokens.CacheRead != 9984 || p.tokens.TotalInput() != 12947 || p.tokens.Output != 5 {
		t.Errorf("tokens = %+v", p.tokens)
	}
	if _, err := parseCodex([]byte("nothing here")); err == nil {
		t.Error("no events should error")
	}
}

func TestParsePiJSON(t *testing.T) {
	stream := strings.Join([]string{
		`{"type":"session","id":"s"}`,
		`{"type":"message_end","message":{"role":"user","content":[{"type":"text","text":"q"}]}}`,
		`{"type":"tool_execution_start","toolName":"bash","args":{"command":"webctl search q --goal g"}}`,
		`{"type":"tool_execution_start","toolName":"bash","args":{"command":"curl x"}}`,
		`{"type":"message_end","message":{"role":"assistant","content":[{"type":"thinking","thinking":"t"},{"type":"text","text":"partial"}],"usage":{"input":700,"output":100,"cacheRead":600,"cacheWrite":0,"cost":{"total":0.004}}}}`,
		`{"type":"message_end","message":{"role":"assistant","content":[{"type":"text","text":"Final"}],"usage":{"input":32,"output":57,"cacheRead":21,"cacheWrite":0,"cost":{"total":0.001}}}}`,
	}, "\n")
	p, err := parsePi([]byte(stream))
	if err != nil {
		t.Fatal(err)
	}
	if p.answer != "Final" || p.turns != 2 || p.webctl != 1 || p.commands != 2 {
		t.Errorf("parsed = %+v", p)
	}
	if p.tokens.Input != 732 || p.tokens.Output != 157 || p.tokens.CacheRead != 621 || p.cost < 0.0049 || p.cost > 0.0051 {
		t.Errorf("tokens = %+v cost %v", p.tokens, p.cost)
	}
}

func TestParseVerdicts(t *testing.T) {
	text := "Here is my grading.\n```json\n{\"answers\":[{\"label\":\"a\",\"score\":8,\"correct\":[\"x\"],\"wrong\":[],\"sourced\":true,\"note\":\"good\"},{\"label\":\"B\",\"score\":13,\"sourced\":false}]}\n```"
	v, err := parseVerdicts(text)
	if err != nil {
		t.Fatal(err)
	}
	if v["A"].Score != 8 || !v["A"].Sourced || v["A"].Note != "good" {
		t.Errorf("A = %+v", v["A"])
	}
	if v["B"].Score != 10 {
		t.Errorf("score should clamp to 10, got %v", v["B"].Score)
	}
	if _, err := parseVerdicts("no json"); err == nil {
		t.Error("should error without JSON")
	}
}

func TestBuildPromptModes(t *testing.T) {
	c := Case{Question: "What is X?"}
	w := BuildPrompt(c, ModeWebctl)
	n := BuildPrompt(c, ModeNative)
	if !strings.Contains(w, "webctl search") || !strings.Contains(w, "--goal") || strings.Contains(w, "built-in web search and web fetch tools, and nothing else") {
		t.Errorf("webctl prompt:\n%s", w)
	}
	if strings.Contains(n, "webctl") || !strings.Contains(n, "Do not run shell commands") {
		t.Errorf("native prompt:\n%s", n)
	}
	if !strings.HasSuffix(strings.TrimSpace(w), "What is X?") || !strings.HasSuffix(strings.TrimSpace(n), "What is X?") {
		t.Error("question must be last in both prompts")
	}
}

func TestPairArmsAndParseArms(t *testing.T) {
	pairs := pairArms(DefaultArms())
	if len(pairs) != 2 {
		t.Fatalf("pairs = %+v", pairs)
	}
	for _, p := range pairs {
		if p.native.Harness != p.webctl.Harness || p.native.Model != p.webctl.Model {
			t.Errorf("mismatched pair %+v", p)
		}
	}
	arms, err := ParseArms("pi-kimi-k3-webctl, claude-sonnet-native")
	if err != nil || len(arms) != 2 || arms[0].Harness != "pi" {
		t.Errorf("arms = %+v, %v", arms, err)
	}
	if _, err := ParseArms("nope"); err == nil {
		t.Error("unknown arm should error")
	}
}

func TestLoadCases(t *testing.T) {
	cases, err := LoadCases("cases", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(cases) < 25 {
		t.Errorf("only %d cases", len(cases))
	}
	domains := map[string]int{}
	for _, c := range cases {
		domains[c.Domain]++
	}
	for _, d := range []string{"dev-docs", "long-doc", "community", "news", "earnings"} {
		if domains[d] == 0 {
			t.Errorf("no %s cases", d)
		}
	}
	sub, err := LoadCases("cases", "kafka-*")
	if err != nil || len(sub) != 2 {
		t.Errorf("glob: %d cases, %v", len(sub), err)
	}
}
