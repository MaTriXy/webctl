// Package jev is a client for TypeSafe's Jev System One API.
package jev

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// QuestionType enumerates the supported System One question types.
type QuestionType string

const (
	TypeScore QuestionType = "score"
	TypeNoul  QuestionType = "noul"
)

// Question is one typed question inside a SystemOneRequest. Score questions
// carry ordered criteria (index 0 = lowest); noul questions omit them.
type Question struct {
	Type         QuestionType `json:"type"`
	Instructions string       `json:"instructions"`
	Criteria     []string     `json:"criteria,omitempty"`
}

// ScoreQuestion builds a score question whose criteria are ordered lowest→highest.
func ScoreQuestion(instructions string, criteria []string) Question {
	return Question{Type: TypeScore, Instructions: instructions, Criteria: criteria}
}

// NoulQuestion builds a yes/no question.
func NoulQuestion(instructions string) Question {
	return Question{Type: TypeNoul, Instructions: instructions}
}

// SystemOneRequest is the POST /v1/systemone body.
type SystemOneRequest struct {
	Model     string              `json:"model"`
	State     any                 `json:"state"`
	Questions map[string]Question `json:"questions"`
}

// Answer is the raw per-question answer. Fields are populated according to Type.
type Answer struct {
	Type QuestionType `json:"type"`

	// Score fields.
	Score         *float64           `json:"score,omitempty"`
	Confidence    *float64           `json:"confidence,omitempty"`
	Legend        map[string]string  `json:"legend,omitempty"`
	Probabilities map[string]float64 `json:"probabilities,omitempty"`

	// Noul field: probability that the answer is "yes".
	Noul *float64 `json:"noul,omitempty"`
}

// Usage reports token consumption for a request.
type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// Add accumulates another usage record into u.
func (u *Usage) Add(o Usage) {
	u.InputTokens += o.InputTokens
	u.OutputTokens += o.OutputTokens
}

// SystemOneResponse is the POST /v1/systemone response.
type SystemOneResponse struct {
	Model   string            `json:"model"`
	Answers map[string]Answer `json:"answers"`
	Usage   Usage             `json:"usage"`
}

// ScoreAnswer is a typed view of a score answer.
type ScoreAnswer struct {
	// Score is the probability-weighted expected level, in [0, len(Legend)-1].
	Score float64 `json:"score"`
	// Confidence is Jev's calibrated confidence in [0, 1].
	Confidence float64 `json:"confidence"`
	// Legend maps level ("0", "1", ...) to its criterion text.
	Legend map[string]string `json:"legend,omitempty"`
	// Probabilities maps level to probability mass.
	Probabilities map[string]float64 `json:"probabilities,omitempty"`
}

// MaxScore returns the highest possible level (len(criteria)-1). It falls
// back to the largest key seen in Probabilities/Legend, or 0 if unknown.
func (s *ScoreAnswer) MaxScore() int {
	max := -1
	for k := range s.Legend {
		if n, err := strconv.Atoi(k); err == nil && n > max {
			max = n
		}
	}
	for k := range s.Probabilities {
		if n, err := strconv.Atoi(k); err == nil && n > max {
			max = n
		}
	}
	if max < 0 {
		return 0
	}
	return max
}

// Level returns the most probable discrete level, or the rounded score if
// probabilities are absent.
func (s *ScoreAnswer) Level() int {
	best, bestP := -1, -1.0
	for k, p := range s.Probabilities {
		n, err := strconv.Atoi(k)
		if err != nil {
			continue
		}
		if p > bestP || (p == bestP && n > best) {
			best, bestP = n, p
		}
	}
	if best >= 0 {
		return best
	}
	return int(s.Score + 0.5)
}

// FormatProbabilities renders probabilities as "{0: 0.01, 1: 0.02, ...}" in level order.
func (s *ScoreAnswer) FormatProbabilities() string {
	keys := make([]int, 0, len(s.Probabilities))
	for k := range s.Probabilities {
		if n, err := strconv.Atoi(k); err == nil {
			keys = append(keys, n)
		}
	}
	sort.Ints(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%d: %.2f", k, s.Probabilities[strconv.Itoa(k)]))
	}
	return "{" + strings.Join(parts, ", ") + "}"
}

// NoulAnswer is a typed view of a yes/no answer.
type NoulAnswer struct {
	// Probability is P(yes) in [0, 1].
	Probability float64 `json:"probability"`
}

// Yes reports whether Jev leans yes (P ≥ 0.5).
func (n *NoulAnswer) Yes() bool { return n.Probability >= 0.5 }

// Confidence is how far the answer is from a coin flip, scaled to [0, 1].
func (n *NoulAnswer) Confidence() float64 {
	c := (n.Probability - 0.5) * 2
	if c < 0 {
		c = -c
	}
	return c
}

// AsScore converts the raw answer to a ScoreAnswer, or errors if the type mismatches.
func (a Answer) AsScore() (*ScoreAnswer, error) {
	if a.Type != TypeScore || a.Score == nil {
		return nil, fmt.Errorf("expected score answer, got type %q", a.Type)
	}
	out := &ScoreAnswer{Score: *a.Score, Legend: a.Legend, Probabilities: a.Probabilities}
	if a.Confidence != nil {
		out.Confidence = *a.Confidence
	}
	return out, nil
}

// AsNoul converts the raw answer to a NoulAnswer, or errors if the type mismatches.
func (a Answer) AsNoul() (*NoulAnswer, error) {
	if a.Type != TypeNoul || a.Noul == nil {
		return nil, fmt.Errorf("expected noul answer, got type %q", a.Type)
	}
	return &NoulAnswer{Probability: *a.Noul}, nil
}
