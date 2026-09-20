// Package benchmarks measures what webctl saves an agent, and what it costs
// in answer quality, by running real agent harnesses (Claude Code, Codex,
// pi) on research questions with and without webctl and having an
// independent model grade the answers.
package benchmarks

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"go.yaml.in/yaml/v3"
)

// Case is one research question an agent must answer from the web.
type Case struct {
	Name   string `yaml:"name"`
	Domain string `yaml:"domain"` // dev-docs, long-doc, community, news, earnings, science, policy
	// Question is given to the agent verbatim.
	Question string `yaml:"question"`
	// Expected are facts a correct answer contains; the judge checks each.
	Expected []string `yaml:"expected"`
	// Rubric are qualities the judge scores when facts alone do not decide
	// (community opinion, recency, sourcing).
	Rubric []string `yaml:"rubric"`
	// Recency marks a question whose true answer changes over time. The
	// judge grades against the best-supported answer across arms plus
	// sourcing, since no fixed expected facts exist.
	Recency bool     `yaml:"recency"`
	Tags    []string `yaml:"tags"`
	// Notes are for humans reading the case file.
	Notes string `yaml:"notes"`
}

// LoadCases reads every *.yaml in dir, sorted by name. pattern, if not
// empty, is a glob matched against the case name.
func LoadCases(dir, pattern string) ([]Case, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var cases []Case
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, err
		}
		var c Case
		if err := yaml.Unmarshal(b, &c); err != nil {
			return nil, fmt.Errorf("%s: %w", e.Name(), err)
		}
		if c.Name == "" {
			c.Name = strings.TrimSuffix(e.Name(), ".yaml")
		}
		if c.Question == "" {
			return nil, fmt.Errorf("%s: question is empty", e.Name())
		}
		if len(c.Expected) == 0 && len(c.Rubric) == 0 && !c.Recency {
			return nil, fmt.Errorf("%s: needs expected facts, a rubric, or recency: true", e.Name())
		}
		if pattern != "" {
			if ok, _ := filepath.Match(pattern, c.Name); !ok {
				continue
			}
		}
		cases = append(cases, c)
	}
	sort.Slice(cases, func(i, j int) bool { return cases[i].Name < cases[j].Name })
	return cases, nil
}
