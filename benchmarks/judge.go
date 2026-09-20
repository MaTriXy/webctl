package benchmarks

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Grade is the judge's verdict on one answer.
type Grade struct {
	// Score is 0–10: 10 is complete, correct, and sourced.
	Score float64 `json:"score"`
	// Correct and Wrong list expected facts (or rubric items) the answer
	// got right and got wrong or missed.
	Correct []string `json:"correct,omitempty"`
	Wrong   []string `json:"wrong,omitempty"`
	// Sourced is whether the answer cites at least one usable URL.
	Sourced bool   `json:"sourced"`
	Note    string `json:"note,omitempty"`
	// Label is the anonymous label the judge saw for this answer.
	Label string `json:"label,omitempty"`
}

// Judge grades every arm's answer to a case in one call, blind to which
// arm wrote which answer. Grading them together lets the judge use
// agreement between answers on recency questions, where no fixed expected
// facts exist, and keeps the scale consistent within a case.
type Judge struct {
	// Model is the pi model pattern; the default is Kimi K3 on Fireworks.
	Model string
	// LogDir receives the judge's raw output per case.
	LogDir string
}

// DefaultJudgeModel is Kimi K3, reached through pi.
const DefaultJudgeModel = "fireworks/accounts/fireworks/models/kimi-k3"

// Grade returns grades aligned with results. Results with an empty answer
// get a zero without being sent.
func (j Judge) Grade(ctx context.Context, c Case, results []Result) ([]Grade, error) {
	grades := make([]Grade, len(results))
	// Shuffle so the judge cannot learn an arm from its position.
	order := rand.Perm(len(results))
	var labeled []int
	for _, i := range order {
		if strings.TrimSpace(results[i].Answer) == "" {
			grades[i] = Grade{Score: 0, Note: "no answer"}
			continue
		}
		labeled = append(labeled, i)
	}
	if len(labeled) == 0 {
		return grades, nil
	}
	labels := make(map[int]string, len(labeled))
	for n, i := range labeled {
		labels[i] = string(rune('A' + n))
	}
	prompt := judgePrompt(c, results, labeled, labels)

	model := j.Model
	if model == "" {
		model = DefaultJudgeModel
	}
	if j.LogDir == "" {
		j.LogDir = os.TempDir()
	}
	_ = os.MkdirAll(j.LogDir, 0o755)
	promptArg, err := piPromptFile(filepath.Join(j.LogDir, c.Name+".judge.prompt.md"), prompt)
	if err != nil {
		return nil, err
	}
	args := append(piArgs(model), "--no-tools", "--thinking", "medium", promptArg)
	cmd := exec.CommandContext(ctx, "pi", args...)
	cmd.Env = hostEnv()
	cmd.Stdin = nil
	out, err := cmd.Output()
	_ = os.WriteFile(filepath.Join(j.LogDir, c.Name+".judge.jsonl"), out, 0o644)
	if err != nil && len(out) == 0 {
		return nil, fmt.Errorf("judge: %w", err)
	}
	parsed, perr := parsePi(out)
	if perr != nil {
		return nil, fmt.Errorf("judge: %w", perr)
	}
	verdicts, verr := parseVerdicts(parsed.answer)
	if verr != nil {
		return nil, fmt.Errorf("judge: %w: %s", verr, firstLine(parsed.answer))
	}
	for _, i := range labeled {
		g, ok := verdicts[labels[i]]
		if !ok {
			return nil, fmt.Errorf("judge: no verdict for answer %s", labels[i])
		}
		g.Label = labels[i]
		grades[i] = g
	}
	return grades, nil
}

func judgePrompt(c Case, results []Result, labeled []int, labels map[int]string) string {
	var b strings.Builder
	b.WriteString("You are grading answers to a research question. Several agents answered the same question; grade each one independently on a 0–10 scale, then return JSON only.\n\n")
	b.WriteString("Scoring: 10 = every expected fact present and correct, nothing wrong, at least one real source URL. ")
	b.WriteString("Deduct for each expected fact missing or wrong. A confidently wrong fact costs more than a missing one. ")
	b.WriteString("An answer that says it could not find something scores low but above a wrong answer. ")
	b.WriteString("Ignore length and style. Do not reward hedging.\n\n")
	if c.Recency {
		b.WriteString("This question's answer changes over time and no fixed expected facts are given. Use agreement between the answers: where several answers independently report the same specific figure, date, or name with a source, treat that as the best-supported answer and grade the others against it. An answer that is specific, dated, and sourced beats one that is vague. An answer whose figure contradicts the best-supported one without a stronger source is wrong.\n\n")
	}
	fmt.Fprintf(&b, "Question: %s\n\n", strings.TrimSpace(c.Question))
	if len(c.Expected) > 0 {
		b.WriteString("Expected facts:\n")
		for _, e := range c.Expected {
			fmt.Fprintf(&b, "- %s\n", e)
		}
		b.WriteString("\n")
	}
	if len(c.Rubric) > 0 {
		b.WriteString("Rubric (each item is a fact-like criterion; treat it like an expected fact):\n")
		for _, r := range c.Rubric {
			fmt.Fprintf(&b, "- %s\n", r)
		}
		b.WriteString("\n")
	}
	for _, i := range labeled {
		fmt.Fprintf(&b, "=== Answer %s ===\n%s\n\n", labels[i], strings.TrimSpace(results[i].Answer))
	}
	b.WriteString("Return exactly this JSON and nothing else:\n")
	b.WriteString(`{"answers":[{"label":"A","score":0,"correct":["..."],"wrong":["..."],"sourced":true,"note":"one sentence"}]}` + "\n")
	b.WriteString("Include one object per answer label. \"correct\" and \"wrong\" list the expected facts or rubric items, quoted briefly, that the answer got right or got wrong/missed.\n")
	return b.String()
}

var jsonBlock = regexp.MustCompile("(?s)\\{.*\\}")

func parseVerdicts(text string) (map[string]Grade, error) {
	m := jsonBlock.FindString(text)
	if m == "" {
		return nil, errors.New("no JSON in judge reply")
	}
	var d struct {
		Answers []struct {
			Label   string   `json:"label"`
			Score   float64  `json:"score"`
			Correct []string `json:"correct"`
			Wrong   []string `json:"wrong"`
			Sourced bool     `json:"sourced"`
			Note    string   `json:"note"`
		} `json:"answers"`
	}
	if err := json.Unmarshal([]byte(m), &d); err != nil {
		return nil, fmt.Errorf("judge JSON: %w", err)
	}
	out := make(map[string]Grade, len(d.Answers))
	for _, a := range d.Answers {
		score := a.Score
		if score < 0 {
			score = 0
		}
		if score > 10 {
			score = 10
		}
		out[strings.ToUpper(strings.TrimSpace(a.Label))] = Grade{
			Score: score, Correct: a.Correct, Wrong: a.Wrong, Sourced: a.Sourced, Note: a.Note,
		}
	}
	if len(out) == 0 {
		return nil, errors.New("judge returned no answers")
	}
	return out, nil
}

// sortedKeys is a small helper for deterministic output.
func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
