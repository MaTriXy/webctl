package benchmarks

import (
	"fmt"
	"io"
	"sort"
	"strings"
)

// armStats aggregates one arm over the cases it completed.
type armStats struct {
	Arm      Arm
	N        int // cells with an answer
	Failed   int
	Judged   int
	Wall     float64 // seconds, summed
	Tokens   Tokens
	Cost     float64
	Score    float64 // summed over judged
	Sourced  int
	Webctl   int
	Searches int
	Violate  int
	Payload  int // chars of search-tool results
}

func (s armStats) meanWall() float64 {
	if s.N == 0 {
		return 0
	}
	return s.Wall / float64(s.N)
}

func (s armStats) meanTokens() float64 {
	if s.N == 0 {
		return 0
	}
	return float64(s.Tokens.Total()) / float64(s.N)
}

func (s armStats) meanScore() float64 {
	if s.Judged == 0 {
		return 0
	}
	return s.Score / float64(s.Judged)
}

func (s armStats) meanCost() float64 {
	if s.N == 0 {
		return 0
	}
	return s.Cost / float64(s.N)
}

func aggregate(run *Run, filter func(Case) bool) map[string]*armStats {
	byName := map[string]*armStats{}
	for _, a := range run.Arms {
		byName[a.Name] = &armStats{Arm: a}
	}
	for _, c := range run.Cases {
		if filter != nil && !filter(c) {
			continue
		}
		for an, r := range run.Results[c.Name] {
			s := byName[an]
			if s == nil {
				continue
			}
			if r.Error != "" && r.Answer == "" {
				s.Failed++
				continue
			}
			s.N++
			s.Wall += float64(r.WallMs) / 1000
			s.Tokens.add(r.Tokens)
			s.Cost += r.CostUSD
			s.Webctl += r.WebctlCalls
			s.Searches += r.SearchCalls
			s.Violate += r.Violations
			s.Payload += r.PayloadChars
			if r.Judge != nil {
				s.Judged++
				s.Score += r.Judge.Score
				if r.Judge.Sourced {
					s.Sourced++
				}
			}
		}
	}
	return byName
}

// WriteReport prints a Markdown report: headline savings per harness,
// per-arm totals, per-domain quality, and a per-case table.
func WriteReport(w io.Writer, run *Run) {
	fmt.Fprintf(w, "# webctl benchmark\n\n")
	fmt.Fprintf(w, "Started %s, webctl %s, judge %s. %d cases × %d arms.\n\n",
		run.Started.Format("2006-01-02 15:04"), run.WebctlVer, run.JudgeModel, len(run.Cases), len(run.Arms))
	fmt.Fprintf(w, "Tokens are everything the model attended to across all turns: uncached input + cache reads + cache writes + output. Cost is what the harness reported (Codex reports none). Quality is the judge's 0–10 score, blind to arm.\n\n")

	all := aggregate(run, nil)

	// Headline: webctl vs native per harness+model.
	fmt.Fprintf(w, "## Savings: webctl vs the harness's own search\n\n")
	fmt.Fprintf(w, "| harness | metric | native | webctl | change |\n|---|---|---|---|---|\n")
	pairs := pairArms(run.Arms)
	for _, p := range pairs {
		n, wc := all[p.native.Name], all[p.webctl.Name]
		label := p.native.Harness + " " + p.native.Model + " → " + string(p.webctl.Mode)
		row := func(metric string, nv, wv float64, format string, lowerIsBetter bool) {
			change := "n/a"
			if nv != 0 {
				pct := (wv - nv) / nv * 100
				change = fmt.Sprintf("%+.0f%%", pct)
			}
			_ = lowerIsBetter
			fmt.Fprintf(w, "| %s | %s | "+format+" | "+format+" | %s |\n", label, metric, nv, wv, change)
		}
		row("wall clock (s, mean)", n.meanWall(), wc.meanWall(), "%.1f", true)
		row("tokens (mean per case)", n.meanTokens(), wc.meanTokens(), "%.0f", true)
		if n.Payload > 0 {
			row("search payload tokens (mean, chars/4)", meanInt(n.Payload, n.N)/4, meanInt(wc.Payload, wc.N)/4, "%.0f", true)
		} else {
			fmt.Fprintf(w, "| %s | search payload tokens (mean, chars/4) | hidden | %.0f | |\n", label, meanInt(wc.Payload, wc.N)/4)
		}
		row("output tokens (mean)", meanInt(n.Tokens.Output, n.N), meanInt(wc.Tokens.Output, wc.N), "%.0f", true)
		if n.Cost > 0 || wc.Cost > 0 {
			row("cost USD (mean)", n.meanCost(), wc.meanCost(), "%.3f", true)
		}
		row("quality (0–10, mean)", n.meanScore(), wc.meanScore(), "%.2f", false)
		fmt.Fprintf(w, "| %s | sourced answers | %d/%d | %d/%d | |\n", label, n.Sourced, n.Judged, wc.Sourced, wc.Judged)
		fmt.Fprintf(w, "| %s | failed runs | %d | %d | |\n", label, n.Failed, wc.Failed)
	}
	fmt.Fprintln(w)

	// Per-arm totals.
	fmt.Fprintf(w, "## Per arm\n\n")
	fmt.Fprintf(w, "| arm | cases | failed | wall s (mean) | tokens (mean) | payload tok (mean) | input | cache read | output | cost USD | quality | sourced | tool calls | violations |\n|---|---|---|---|---|---|---|---|---|---|---|---|---|---|\n")
	for _, a := range run.Arms {
		s := all[a.Name]
		calls := fmt.Sprintf("%d webctl", s.Webctl)
		if a.Mode == ModeNative {
			calls = fmt.Sprintf("%d search", s.Searches)
		}
		fmt.Fprintf(w, "| %s | %d | %d | %.1f | %.0f | %.0f | %d | %d | %d | %.3f | %.2f | %d/%d | %s | %d |\n",
			a.Name, s.N, s.Failed, s.meanWall(), s.meanTokens(), meanInt(s.Payload, s.N)/4, s.Tokens.Input+s.Tokens.CacheWrite, s.Tokens.CacheRead, s.Tokens.Output,
			s.Cost, s.meanScore(), s.Sourced, s.Judged, calls, s.Violate)
	}
	fmt.Fprintln(w)

	// Per domain quality and tokens.
	domains := map[string]bool{}
	for _, c := range run.Cases {
		domains[c.Domain] = true
	}
	fmt.Fprintf(w, "## Per domain (quality mean / tokens mean)\n\n| domain | cases |")
	for _, a := range run.Arms {
		fmt.Fprintf(w, " %s |", a.Name)
	}
	fmt.Fprintf(w, "\n|---|---|")
	for range run.Arms {
		fmt.Fprintf(w, "---|")
	}
	fmt.Fprintln(w)
	for _, d := range sortedKeys(domains) {
		dd := d
		st := aggregate(run, func(c Case) bool { return c.Domain == dd })
		n := 0
		for _, c := range run.Cases {
			if c.Domain == d {
				n++
			}
		}
		fmt.Fprintf(w, "| %s | %d |", d, n)
		for _, a := range run.Arms {
			s := st[a.Name]
			fmt.Fprintf(w, " %.1f / %.0fk |", s.meanScore(), s.meanTokens()/1000)
		}
		fmt.Fprintln(w)
	}
	fmt.Fprintln(w)

	// Per case.
	fmt.Fprintf(w, "## Per case (quality · seconds · tokens)\n\n| case | domain |")
	for _, a := range run.Arms {
		fmt.Fprintf(w, " %s |", a.Name)
	}
	fmt.Fprintf(w, "\n|---|---|")
	for range run.Arms {
		fmt.Fprintf(w, "---|")
	}
	fmt.Fprintln(w)
	for _, c := range run.Cases {
		fmt.Fprintf(w, "| %s | %s |", c.Name, c.Domain)
		for _, a := range run.Arms {
			r := run.Results[c.Name][a.Name]
			switch {
			case r == nil:
				fmt.Fprintf(w, " – |")
			case r.Error != "" && r.Answer == "":
				fmt.Fprintf(w, " FAIL |")
			case r.Judge == nil:
				fmt.Fprintf(w, " ? · %.0fs · %dk |", float64(r.WallMs)/1000, r.Tokens.Total()/1000)
			default:
				fmt.Fprintf(w, " %.0f · %.0fs · %dk |", r.Judge.Score, float64(r.WallMs)/1000, r.Tokens.Total()/1000)
			}
		}
		fmt.Fprintln(w)
	}
	fmt.Fprintln(w)

	// Where webctl lost quality.
	fmt.Fprintf(w, "## Quality losses and wins with webctl (score difference ≥ 2)\n\n")
	any := false
	for _, p := range pairs {
		for _, c := range run.Cases {
			n, wc := run.Results[c.Name][p.native.Name], run.Results[c.Name][p.webctl.Name]
			if n == nil || wc == nil || n.Judge == nil || wc.Judge == nil {
				continue
			}
			d := wc.Judge.Score - n.Judge.Score
			if d >= 2 || d <= -2 {
				any = true
				fmt.Fprintf(w, "- **%s** (%s): native %.0f → webctl %.0f. webctl judge note: %s. native judge note: %s\n",
					c.Name, p.native.Harness, n.Judge.Score, wc.Judge.Score, wc.Judge.Note, n.Judge.Note)
			}
		}
	}
	if !any {
		fmt.Fprintln(w, "None.")
	}
	fmt.Fprintln(w)

	// Failures.
	var fails []string
	for _, c := range run.Cases {
		for _, a := range run.Arms {
			if r := run.Results[c.Name][a.Name]; r != nil && r.Error != "" {
				fails = append(fails, fmt.Sprintf("- %s / %s: %s", c.Name, a.Name, r.Error))
			}
		}
	}
	if len(fails) > 0 {
		fmt.Fprintf(w, "## Failures\n\n%s\n\n", strings.Join(fails, "\n"))
	}
}

type armPair struct{ native, webctl Arm }

// pairArms matches native and webctl arms of the same harness and model.
func pairArms(arms []Arm) []armPair {
	var pairs []armPair
	for _, n := range arms {
		if n.Mode != ModeNative {
			continue
		}
		for _, wc := range arms {
			if wc.Mode.UsesWebctl() && wc.Harness == n.Harness && wc.Model == n.Model {
				pairs = append(pairs, armPair{native: n, webctl: wc})
			}
		}
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].native.Name != pairs[j].native.Name {
			return pairs[i].native.Name < pairs[j].native.Name
		}
		return pairs[i].webctl.Name < pairs[j].webctl.Name
	})
	return pairs
}

func meanInt(sum, n int) float64 {
	if n == 0 {
		return 0
	}
	return float64(sum) / float64(n)
}
