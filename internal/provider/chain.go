package provider

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

// Chain timing: one attempt is bounded by AttemptTimeout so a hung backend
// cannot eat the run, and the chain as a whole by Budget so a run of slow
// failures still ends promptly.
const (
	DefaultAttemptTimeout = 12 * time.Second
	DefaultChainBudget    = 30 * time.Second
)

// Chain tries providers in order and returns the first useful answer. It
// is itself a Provider, so it drops into any single-provider call site.
// Providers are constructed lazily, so a provider later in the chain that
// cannot be built (a missing key, say) only matters if it is reached.
type Chain struct {
	Names []string
	New   func(name string) (Provider, error)
	// AttemptTimeout and Budget default to the package constants when ≤ 0.
	AttemptTimeout time.Duration
	Budget         time.Duration
	// OnFallthrough, if set, is told about each provider that failed and
	// which one is tried next.
	OnFallthrough func(failed string, err error, next string)
	// NoTopUp disables topping up a short answer from the next provider.
	NoTopUp bool
	// Cooldowns, when set, skips providers that recently rate limited.
	// CooldownKey maps a provider name to its cooldown entry (keyed and
	// keyless use of the same provider are tracked apart); nil uses the name.
	Cooldowns   Cooldowns
	CooldownKey func(name string) string
	// OnSkip, if set, is told when a cooling-down provider is skipped and
	// whether the user should hear about it this time.
	OnSkip func(name, reason string, announce bool)
	// Sources is how many providers to gather results from. 1 (or 0) is
	// the classic fallback chain; more queries that many providers in
	// concurrent waves, refilling from the next names when one fails, and
	// fuses their lists by reciprocal rank.
	Sources int

	// answered is the provider (or fused set) that served the last search.
	answered string
	// engines records, after a fused search, which providers returned each URL.
	engines map[string][]string
}

// Engines returns, for each URL of the last fused search, the providers
// that returned it. Nil after a single-source search.
func (c *Chain) Engines() map[string][]string { return c.engines }

// topUpFraction is the share of the requested count below which an answer
// is short enough to top up from the next provider. Keyless tiers that are
// throttled tend to answer with a truncated, lower-quality list rather than
// an error. A top-up is optional, so it gets at most topUpTimeout.
const (
	topUpFraction = 0.5
	topUpTimeout  = 4 * time.Second
)

// Cooldowns is what a Chain consults before and after each provider call.
// *Cooldown implements it; nil means never skip.
type Cooldowns interface {
	Skip(name string) (skip bool, reason string, announce bool)
	Note(name string, err error)
}

// errCoolingDown marks a provider skipped because of a recent 429.
var errCoolingDown = errors.New("rate limited recently; skipped")

// Name returns the provider that answered the last search, or the first
// name before any search.
func (c *Chain) Name() string {
	if c.answered != "" {
		return c.answered
	}
	if len(c.Names) > 0 {
		return c.Names[0]
	}
	return "chain"
}

// Search tries each provider until one returns results. An empty answer
// only counts when no provider is left, since a keyless tier that is
// throttled often answers with nothing rather than an error.
func (c *Chain) Search(ctx context.Context, query string, numResults int) ([]SearchResult, error) {
	if len(c.Names) == 0 || c.New == nil {
		return nil, errors.New("chain: no providers")
	}
	c.engines = nil
	if c.Sources > 1 {
		return c.searchSources(ctx, query, numResults)
	}
	attempt, budget := c.AttemptTimeout, c.Budget
	if attempt <= 0 {
		attempt = DefaultAttemptTimeout
	}
	if budget <= 0 {
		budget = DefaultChainBudget
	}
	chainCtx, cancelChain := context.WithTimeout(ctx, budget)
	defer cancelChain()

	var errs []error
	for i, name := range c.Names {
		last := i+1 == len(c.Names)
		var p Provider
		err := errCoolingDown
		if !c.coolingDown(name) {
			p, err = c.New(name)
		}
		if err == nil {
			var results []SearchResult
			attemptCtx, cancel := context.WithTimeout(chainCtx, attempt)
			results, err = p.Search(attemptCtx, query, numResults)
			cancel()
			c.note(name, err)
			if err == nil && (len(results) > 0 || last) {
				c.answered = p.Name()
				if !last && !c.NoTopUp && len(results) < int(float64(numResults)*topUpFraction) {
					results = c.topUp(chainCtx, attempt, i+1, query, numResults, Ranked{Engine: p.Name(), Results: results})
				}
				return results, nil
			}
			if err == nil {
				err = errors.New("no results")
			}
		}
		errs = append(errs, err)
		if ctx.Err() != nil {
			break
		}
		if chainCtx.Err() != nil {
			errs = append(errs, fmt.Errorf("chain budget of %s exhausted", budget))
			break
		}
		if !last && c.OnFallthrough != nil && !errors.Is(err, errCoolingDown) {
			c.OnFallthrough(name, err, c.Names[i+1])
		}
	}
	if len(errs) == 1 {
		return nil, errs[0]
	}
	return nil, fmt.Errorf("all %d providers failed: %w", len(c.Names), errors.Join(errs...))
}

// topUp asks the next provider that can be built for the same query and
// fuses its list with first by reciprocal rank. Any failure leaves first
// as it was. The fused engine names become the chain's Name.
func (c *Chain) topUp(ctx context.Context, attempt time.Duration, from int, query string, numResults int, first Ranked) []SearchResult {
	for _, name := range c.Names[from:] {
		if c.coolingDown(name) {
			continue
		}
		p, err := c.New(name)
		if err != nil {
			continue
		}
		attemptCtx, cancel := context.WithTimeout(ctx, min(attempt, topUpTimeout))
		more, err := p.Search(attemptCtx, query, numResults)
		cancel()
		c.note(name, err)
		if err != nil || len(more) == 0 {
			continue
		}
		fused := Fuse([]Ranked{first, {Engine: p.Name(), Results: more}}, RRFK, numResults)
		out := make([]SearchResult, 0, len(fused))
		for _, f := range fused {
			out = append(out, f.SearchResult)
		}
		c.answered = first.Engine + "+" + p.Name()
		return out
	}
	return first.Results
}

// searchSources gathers up to c.Sources provider lists. Providers are
// tried in chain order in waves of the still-needed count: a wave's
// failures (errors, empty answers, cooldowns) are replaced from the next
// names until enough lists are in hand or the names run out. The lists
// are fused by reciprocal rank; a single successful list is returned as
// is.
func (c *Chain) searchSources(ctx context.Context, query string, numResults int) ([]SearchResult, error) {
	attempt, budget := c.AttemptTimeout, c.Budget
	if attempt <= 0 {
		attempt = DefaultAttemptTimeout
	}
	if budget <= 0 {
		budget = DefaultChainBudget
	}
	chainCtx, cancelChain := context.WithTimeout(ctx, budget)
	defer cancelChain()

	type outcome struct {
		name    string
		results []SearchResult
		err     error
	}
	var lists []Ranked
	var errs []error
	next := 0
	for len(lists) < c.Sources && next < len(c.Names) && chainCtx.Err() == nil {
		// Pick the next providers that are not cooling down.
		var wave []string
		for len(wave) < c.Sources-len(lists) && next < len(c.Names) {
			name := c.Names[next]
			next++
			if c.coolingDown(name) {
				errs = append(errs, fmt.Errorf("%s: %w", name, errCoolingDown))
				continue
			}
			wave = append(wave, name)
		}
		if len(wave) == 0 {
			break
		}
		outcomes := make([]outcome, len(wave))
		var wg sync.WaitGroup
		for i, name := range wave {
			wg.Add(1)
			go func(i int, name string) {
				defer wg.Done()
				p, err := c.New(name)
				if err != nil {
					outcomes[i] = outcome{name: name, err: err}
					return
				}
				attemptCtx, cancel := context.WithTimeout(chainCtx, attempt)
				results, err := p.Search(attemptCtx, query, numResults)
				cancel()
				c.note(name, err)
				if err == nil && len(results) == 0 {
					err = errors.New("no results")
				}
				outcomes[i] = outcome{name: name, results: results, err: err}
			}(i, name)
		}
		wg.Wait()
		for _, o := range outcomes {
			if o.err != nil {
				errs = append(errs, fmt.Errorf("%s: %w", o.name, o.err))
				if c.OnFallthrough != nil {
					nextName := ""
					if next < len(c.Names) {
						nextName = c.Names[next]
					}
					c.OnFallthrough(o.name, o.err, nextName)
				}
				continue
			}
			lists = append(lists, Ranked{Engine: o.name, Results: o.results})
		}
	}
	if len(lists) == 0 {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("search cancelled after %d failures: %w", len(errs), ctx.Err())
		}
		if len(errs) == 1 {
			return nil, errs[0]
		}
		return nil, fmt.Errorf("all %d providers failed: %w", len(errs), errors.Join(errs...))
	}
	names := make([]string, len(lists))
	for i, l := range lists {
		names[i] = l.Engine
	}
	c.answered = strings.Join(names, "+")
	if len(lists) == 1 {
		if numResults > 0 && len(lists[0].Results) > numResults {
			return lists[0].Results[:numResults], nil
		}
		return lists[0].Results, nil
	}
	fused := Fuse(lists, RRFK, numResults)
	out := make([]SearchResult, 0, len(fused))
	c.engines = make(map[string][]string, len(fused))
	for _, f := range fused {
		out = append(out, f.SearchResult)
		c.engines[f.URL] = f.Engines
	}
	return out, nil
}

func (c *Chain) cooldownKey(name string) string {
	if c.CooldownKey != nil {
		return c.CooldownKey(name)
	}
	return name
}

// coolingDown consults Cooldowns and reports a skip to OnSkip.
func (c *Chain) coolingDown(name string) bool {
	if c.Cooldowns == nil {
		return false
	}
	skip, reason, announce := c.Cooldowns.Skip(c.cooldownKey(name))
	if skip && c.OnSkip != nil {
		c.OnSkip(name, reason, announce)
	}
	return skip
}

func (c *Chain) note(name string, err error) {
	if c.Cooldowns != nil {
		c.Cooldowns.Note(c.cooldownKey(name), err)
	}
}

// Validate validates the first provider.
func (c *Chain) Validate(ctx context.Context) error {
	if len(c.Names) == 0 || c.New == nil {
		return errors.New("chain: no providers")
	}
	p, err := c.New(c.Names[0])
	if err != nil {
		return err
	}
	return p.Validate(ctx)
}
