package provider

import (
	"context"
	"errors"
	"fmt"
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

	// answered is the provider that served the last successful search.
	answered string
}

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
		p, err := c.New(name)
		if err == nil {
			var results []SearchResult
			attemptCtx, cancel := context.WithTimeout(chainCtx, attempt)
			results, err = p.Search(attemptCtx, query, numResults)
			cancel()
			if err == nil && (len(results) > 0 || last) {
				c.answered = p.Name()
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
