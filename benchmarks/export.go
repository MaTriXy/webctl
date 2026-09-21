package benchmarks

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Experiments live in the repo so evidence outlives ~/webctl. Layout:
//
//	benchmarks/experiments/<name>/
//	  README.md            what was tested and the headline
//	  results/meta.json    run metadata: when, webctl version, judge, arms, cases
//	  results/cells.jsonl  one line per case × arm: the full Result plus case
//	                       domain, tags, and the prompt mode
//	  results/report.md    the generated report
//
// cells.jsonl is the durable record; everything in the report can be
// recomputed from it.

// Cell is one line of cells.jsonl.
type Cell struct {
	Experiment string   `json:"experiment"`
	Case       string   `json:"case"`
	Domain     string   `json:"domain"`
	Tags       []string `json:"tags,omitempty"`
	Recency    bool     `json:"recency,omitempty"`
	Arm        string   `json:"arm"`
	Harness    string   `json:"harness"`
	Model      string   `json:"model"`
	Mode       Mode     `json:"mode"`
	WebctlVer  string   `json:"webctl_version"`
	Result
}

// Export writes run into dir as an experiment. Cells missing a payload
// measurement are backfilled from their logs when those still exist.
func Export(run *Run, name, dir string) error {
	resDir := filepath.Join(dir, "results")
	if err := os.MkdirAll(resDir, 0o755); err != nil {
		return err
	}
	BackfillPayload(run)

	f, err := os.Create(filepath.Join(resDir, "cells.jsonl"))
	if err != nil {
		return err
	}
	w := bufio.NewWriter(f)
	enc := json.NewEncoder(w)
	armByName := map[string]Arm{}
	for _, a := range run.Arms {
		armByName[a.Name] = a
	}
	for _, c := range run.Cases {
		for _, a := range run.Arms {
			r := run.Results[c.Name][a.Name]
			if r == nil {
				continue
			}
			cell := Cell{
				Experiment: name, Case: c.Name, Domain: c.Domain, Tags: c.Tags, Recency: c.Recency,
				Arm: a.Name, Harness: a.Harness, Model: a.Model, Mode: a.Mode, WebctlVer: run.WebctlVer,
				Result: *r,
			}
			cell.Log = "" // machine-local path; not durable
			if err := enc.Encode(cell); err != nil {
				return err
			}
		}
	}
	if err := w.Flush(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}

	meta := map[string]any{
		"experiment":     name,
		"started":        run.Started,
		"finished":       run.Finished,
		"exported":       time.Now(),
		"webctl_version": run.WebctlVer,
		"judge_model":    run.JudgeModel,
		"arms":           run.Arms,
		"cases":          len(run.Cases),
		"case_names":     caseNames(run.Cases),
	}
	b, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(resDir, "meta.json"), b, 0o644); err != nil {
		return err
	}
	rep, err := os.Create(filepath.Join(resDir, "report.md"))
	if err != nil {
		return err
	}
	WriteReport(rep, run)
	return rep.Close()
}

// LoadExperiment reads cells.jsonl back into a Run so `report` works on
// an exported experiment as well as on a run file.
func LoadExperiment(dir string) (*Run, error) {
	f, err := os.Open(filepath.Join(dir, "results", "cells.jsonl"))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	run := &Run{Results: map[string]map[string]*Result{}}
	seenArm := map[string]bool{}
	seenCase := map[string]bool{}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 64<<20)
	for sc.Scan() {
		var cell Cell
		if err := json.Unmarshal(sc.Bytes(), &cell); err != nil {
			return nil, err
		}
		if !seenArm[cell.Arm] {
			seenArm[cell.Arm] = true
			run.Arms = append(run.Arms, Arm{Name: cell.Arm, Harness: cell.Harness, Model: cell.Model, Mode: cell.Mode})
		}
		if !seenCase[cell.Case] {
			seenCase[cell.Case] = true
			run.Cases = append(run.Cases, Case{Name: cell.Case, Domain: cell.Domain, Tags: cell.Tags, Recency: cell.Recency})
			run.Results[cell.Case] = map[string]*Result{}
		}
		r := cell.Result
		run.Results[cell.Case][cell.Arm] = &r
		run.WebctlVer = cell.WebctlVer
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if b, err := os.ReadFile(filepath.Join(dir, "results", "meta.json")); err == nil {
		var meta struct {
			Started    time.Time `json:"started"`
			JudgeModel string    `json:"judge_model"`
		}
		if json.Unmarshal(b, &meta) == nil {
			run.Started, run.JudgeModel = meta.Started, meta.JudgeModel
		}
	}
	return run, nil
}

// BackfillPayload fills PayloadChars for cells recorded before payload was
// measured, by re-parsing their logs. Cells whose log is gone are left at 0.
func BackfillPayload(run *Run) {
	armByName := map[string]Arm{}
	for _, a := range run.Arms {
		armByName[a.Name] = a
	}
	for _, arms := range run.Results {
		for an, r := range arms {
			if r == nil || r.PayloadChars > 0 || r.Log == "" {
				continue
			}
			b, err := os.ReadFile(r.Log)
			if err != nil {
				continue
			}
			switch armByName[an].Harness {
			case "claude":
				if p, err := parseClaude(b); err == nil {
					r.PayloadChars = p.payload
				}
			case "codex":
				if p, err := parseCodex(b); err == nil {
					r.PayloadChars = p.payload
				}
			case "pi":
				if p, err := parsePi(b); err == nil {
					r.PayloadChars = p.payload
				}
			}
		}
	}
}

func caseNames(cases []Case) []string {
	out := make([]string, len(cases))
	for i, c := range cases {
		out[i] = c.Name
	}
	return out
}

// ExperimentName makes a directory-safe name.
func ExperimentName(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-', r == '.':
			b.WriteRune(r)
		case r == ' ' || r == '_' || r == '/':
			b.WriteRune('-')
		}
	}
	if b.Len() == 0 {
		return fmt.Sprintf("experiment-%s", time.Now().Format("2006-01-02"))
	}
	return b.String()
}
