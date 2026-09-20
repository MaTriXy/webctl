package provider

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCooldownLadderProbeAndPersistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cooldown.json")
	clock := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)
	cfg := CooldownConfig{Enabled: true, Steps: []time.Duration{time.Minute, 10 * time.Minute, time.Hour}, ProbeInterval: 40 * time.Minute, QuotaStart: 2}
	c := NewCooldown(path, cfg)
	c.Now = func() time.Time { return clock }

	if skip, _, _ := c.Skip("exa"); skip {
		t.Fatal("fresh store should not skip")
	}
	c.Note("exa", &APIError{Provider: "Exa", Status: 429})
	skip, reason, announce := c.Skip("exa")
	if !skip || !announce || !strings.Contains(reason, "strike 1 of 3") || !strings.Contains(reason, "cooldown clear exa") {
		t.Fatalf("after first 429: %v %q %v", skip, reason, announce)
	}
	if _, _, announce := c.Skip("exa"); announce {
		t.Error("second skip within the hour should be quiet")
	}
	// A new process reads the same file.
	c2 := NewCooldown(path, cfg)
	c2.Now = c.Now
	if skip, _, _ := c2.Skip("exa"); !skip {
		t.Error("cooldown should persist across instances")
	}
	// Window passes; the next failure climbs the ladder.
	clock = clock.Add(2 * time.Minute)
	if skip, _, _ := c.Skip("exa"); skip {
		t.Error("window elapsed; should try again")
	}
	c.Note("exa", &APIError{Provider: "Exa", Status: 429})
	e := c.Entries()["exa"]
	if e.Strikes != 2 || e.Until.Sub(clock) != 10*time.Minute {
		t.Errorf("second strike = %+v", e)
	}
	clock = clock.Add(11 * time.Minute)
	c.Note("exa", &APIError{Provider: "Exa", Status: 429})
	if e := c.Entries()["exa"]; e.Strikes != 3 || e.Until.Sub(clock) != time.Hour {
		t.Errorf("third strike = %+v", e)
	}
	// At the ceiling, one probe per ProbeInterval is allowed and a failure re-arms without exceeding the ladder.
	clock = clock.Add(30 * time.Minute)
	if skip, _, _ := c.Skip("exa"); !skip {
		t.Error("inside the ceiling window and probe interval: should skip")
	}
	clock = clock.Add(15 * time.Minute) // 45m after the strike: inside the 1h window, past the 40m probe interval
	if skip, _, _ := c.Skip("exa"); skip {
		t.Error("probe interval elapsed: should allow a probe")
	}
	if skip, _, _ := c.Skip("exa"); !skip {
		t.Error("only one probe per interval")
	}
	c.Note("exa", &APIError{Provider: "Exa", Status: 429})
	if e := c.Entries()["exa"]; e.Strikes != 4 || e.Until.Sub(clock) != time.Hour {
		t.Errorf("probe failure = %+v", e)
	}
	// Success clears.
	c.Note("exa", nil)
	if _, ok := c.Entries()["exa"]; ok {
		t.Error("success should clear the entry")
	}
	// 402 starts at QuotaStart; other errors do not count; Clear works.
	c.Note("youcom", &APIError{Provider: "You.com", Status: 402})
	if e := c.Entries()["youcom"]; e.Strikes != 2 {
		t.Errorf("402 should start at strike 2: %+v", e)
	}
	c.Note("parallel", errors.New("transport error"))
	if _, ok := c.Entries()["parallel"]; ok {
		t.Error("a transport error is not a cooldown")
	}
	if got := c.Clear("youcom"); len(got) != 1 || c.Active("youcom") {
		t.Errorf("clear = %v", got)
	}
	// Disabled store never skips.
	off := NewCooldown("", CooldownConfig{Enabled: false, Steps: cfg.Steps, ProbeInterval: time.Hour, QuotaStart: 1})
	off.Note("exa", &APIError{Status: 429})
	if skip, _, _ := off.Skip("exa"); skip {
		t.Error("disabled cooldown skipped")
	}
}
