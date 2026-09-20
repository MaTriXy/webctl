// webctl-bench runs real agent harnesses on research questions with and
// without webctl and reports wall clock, tokens, cost, and judged quality.
package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/dorkitude/webctl/benchmarks"
)

func main() {
	if err := newRoot().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func defaultRunsDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "webctl", "benchmarks")
}

func newRoot() *cobra.Command {
	root := &cobra.Command{
		Use:   "webctl-bench",
		Short: "Benchmark agents with and without webctl",
		Long: `Runs Claude Code, Codex, and pi on the questions in benchmarks/cases,
once told to use only webctl and (where the harness has its own search)
once told to use only that. Kimi K3, via pi, grades every answer blind.

Runs are saved as JSON under ~/webctl/benchmarks with raw logs beside them.`,
		SilenceUsage: true,
	}
	root.AddCommand(newRunCmd(), newReportCmd(), newListCmd())
	return root
}

func newRunCmd() *cobra.Command {
	var (
		casesDir, pattern, armSpec, out, resume, judgeModel string
		concurrency                                         int
		timeout                                             time.Duration
		noJudge                                             bool
	)
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run the matrix and grade the answers",
		RunE: func(cmd *cobra.Command, args []string) error {
			cases, err := benchmarks.LoadCases(casesDir, pattern)
			if err != nil {
				return err
			}
			if len(cases) == 0 {
				return fmt.Errorf("no cases match %q in %s", pattern, casesDir)
			}
			arms, err := benchmarks.ParseArms(armSpec)
			if err != nil {
				return err
			}
			for _, h := range []string{"claude", "codex", "pi"} {
				needed := false
				for _, a := range arms {
					if a.Harness == h {
						needed = true
					}
				}
				if needed || (!noJudge && h == "pi") {
					if _, err := exec.LookPath(h); err != nil {
						return fmt.Errorf("%s is not on PATH", h)
					}
				}
			}
			if _, err := exec.LookPath("webctl"); err != nil {
				return fmt.Errorf("webctl is not on PATH")
			}
			ver, _ := exec.Command("webctl", "--version").Output()

			var prior *benchmarks.Run
			if resume != "" {
				if resume == "latest" {
					resume, err = benchmarks.Latest(defaultRunsDir())
					if err != nil {
						return err
					}
				}
				prior, err = benchmarks.LoadRun(resume)
				if err != nil {
					return err
				}
				if out == "" {
					out = resume
				}
			}
			if out == "" {
				if err := os.MkdirAll(defaultRunsDir(), 0o755); err != nil {
					return err
				}
				out = filepath.Join(defaultRunsDir(), time.Now().Format("2006-01-02T15-04-05")+".json")
			}
			var judge *benchmarks.Judge
			if !noJudge {
				judge = &benchmarks.Judge{Model: judgeModel, LogDir: filepath.Join(strings.TrimSuffix(out, ".json"), "judge")}
			}
			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
			defer stop()
			fmt.Fprintf(os.Stderr, "%d cases × %d arms → %s\n", len(cases), len(arms), out)
			run, err := benchmarks.Execute(ctx, benchmarks.Options{
				Arms:          arms,
				Cases:         cases,
				Concurrency:   concurrency,
				Timeout:       timeout,
				Judge:         judge,
				OutPath:       out,
				Resume:        prior,
				Progress:      os.Stderr,
				WebctlVersion: strings.TrimSpace(strings.TrimPrefix(string(ver), "webctl version ")),
			})
			if err != nil {
				return err
			}
			fmt.Fprintf(os.Stderr, "saved %s\n\n", out)
			benchmarks.WriteReport(os.Stdout, run)
			return nil
		},
	}
	f := cmd.Flags()
	f.StringVar(&casesDir, "cases-dir", "benchmarks/cases", "directory of case YAML files")
	f.StringVar(&pattern, "cases", "", "glob on case names, e.g. 'kafka-*'")
	f.StringVar(&armSpec, "arms", "all", "comma-separated arm names, or all: "+armNames())
	f.StringVar(&out, "out", "", "run file to write (default ~/webctl/benchmarks/<time>.json)")
	f.StringVar(&resume, "resume", "", "run file (or 'latest') whose finished cells are kept; only missing cells run")
	f.StringVar(&judgeModel, "judge-model", benchmarks.DefaultJudgeModel, "pi model pattern for the judge")
	f.IntVar(&concurrency, "concurrency", 3, "agent runs in flight at once (each harness gets half)")
	f.DurationVar(&timeout, "timeout", 6*time.Minute, "per agent run")
	f.BoolVar(&noJudge, "no-judge", false, "skip grading")
	return cmd
}

func newReportCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "report [run.json|latest]",
		Short: "Print the Markdown report for a saved run",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := "latest"
			if len(args) == 1 {
				path = args[0]
			}
			if path == "latest" {
				var err error
				path, err = benchmarks.Latest(defaultRunsDir())
				if err != nil {
					return err
				}
			}
			run, err := benchmarks.LoadRun(path)
			if err != nil {
				return err
			}
			benchmarks.WriteReport(os.Stdout, run)
			return nil
		},
	}
}

func newListCmd() *cobra.Command {
	var casesDir string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List cases and arms",
		RunE: func(cmd *cobra.Command, args []string) error {
			cases, err := benchmarks.LoadCases(casesDir, "")
			if err != nil {
				return err
			}
			fmt.Printf("%d cases:\n", len(cases))
			for _, c := range cases {
				kind := "facts"
				if c.Recency {
					kind = "recency"
				} else if len(c.Expected) == 0 {
					kind = "rubric"
				}
				fmt.Printf("  %-28s %-10s %-8s %s\n", c.Name, c.Domain, kind, truncate(c.Question, 70))
			}
			fmt.Println("\narms:")
			for _, a := range benchmarks.DefaultArms() {
				fmt.Printf("  %-24s %s %s (%s)\n", a.Name, a.Harness, a.Model, a.Mode)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&casesDir, "cases-dir", "benchmarks/cases", "directory of case YAML files")
	return cmd
}

func armNames() string {
	var names []string
	for _, a := range benchmarks.DefaultArms() {
		names = append(names, a.Name)
	}
	return strings.Join(names, ",")
}

func truncate(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}
