package benchmarks

import (
	"context"
	"os"
	"testing"
)

// TestPiLive runs the real pi harness once; set BENCH_LIVE=1 to enable.
func TestPiLive(t *testing.T) {
	if os.Getenv("BENCH_LIVE") == "" {
		t.Skip("BENCH_LIVE not set")
	}
	cases, err := LoadCases("cases", "redis-xautoclaim")
	if err != nil {
		t.Fatal(err)
	}
	c := cases[0]
	arm := DefaultArms()[4]
	work := t.TempDir()
	res, err := piRunner{}.Run(context.Background(), arm, BuildPrompt(c, ModeWebctl), work, t.TempDir())
	if err != nil {
		t.Fatalf("pi: %v", err)
	}
	t.Logf("answer=%q tokens=%+v webctl=%d", res.Answer, res.Tokens, res.WebctlCalls)
}

func TestPiLiveViaExecute(t *testing.T) {
	if os.Getenv("BENCH_LIVE") == "" {
		t.Skip("BENCH_LIVE not set")
	}
	c := Case{Name: "live", Question: "In which Redis version was XAUTOCLAIM introduced?", Expected: []string{"6.2"}}
	out := t.TempDir() + "/run.json"
	run, err := Execute(context.Background(), Options{Arms: []Arm{DefaultArms()[4]}, Cases: []Case{c}, OutPath: out, Progress: os.Stderr})
	if err != nil {
		t.Fatal(err)
	}
	r := run.Results["live"]["pi-kimi-k3-webctl"]
	if r.Error != "" {
		t.Fatalf("cell failed: %s", r.Error)
	}
}
