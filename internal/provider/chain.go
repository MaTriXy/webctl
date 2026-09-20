package provider

import (
	"context"
	"errors"
	"fmt"
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

	// answered is the provider (or fused pair) that served the last search.
	answered string
}

// topUpFraction is the share of the requested count below which an answer
// is short enough to top up from the next provider. Keyless tiers that are
// throttled tend to answer with a truncated, lower-quality list rather than
// an error. A top-up is optional, so it gets at most topUpTimeout.
const (
	topUpFraction = 0.5
	topUpTimeout  = 4 * time.Second
)

// rateLimitCooldown is how long a provider that answered 429 is skipped by
// every Chain in this process. Free tiers meter by the minute; asking again
// sooner only spends a request and a fall-through.
const rateLimitCooldown = 90 * time.Second

var (
	cooldownMu    sync.Mutex
	cooldownUntil = map[string]time.Time{}
	now           = time.Now
)

// coolingDown reports whether name answered 429 recently.
func coolingDown(name string) bool {
	cooldownMu.Lock()
	defer cooldownMu.Unlock()
	return now().Before(cooldownUntil[name])
}

// noteRateLimit starts the cooldown for name if err is a 429 or a 402
// (You.com's free profile answers 402 once its quota is spent).
func noteRateLimit(name string, err error) {
	var apiErr *APIError
	if !errors.As(err, &apiErr) || (apiErr.Status != 429 && apiErr.Status != 402) {
		return
	}
	cooldownMu.Lock()
	cooldownUntil[name] = now().Add(rateLimitCooldown)
	cooldownMu.Unlock()
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
		if !coolingDown(name) {
			p, err = c.New(name)
		}
		if err == nil {
			var results []SearchResult
			attemptCtx, cancel := context.WithTimeout(chainCtx, attempt)
			results, err = p.Search(attemptCtx, query, numResults)
			cancel()
			noteRateLimit(name, err)
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
		if !last && c.OnFallthrough != nil {
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
		if coolingDown(name) {
			continue
		}
		p, err := c.New(name)
		if err != nil {
			continue
		}
		attemptCtx, cancel := context.WithTimeout(ctx, min(attempt, topUpTimeout))
		more, err := p.Search(attemptCtx, query, numResults)
		cancel()
		noteRateLimit(name, err)
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
