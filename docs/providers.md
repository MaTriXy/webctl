# Providers

Six search backends; three need no key. Every search gathers from several of them. (Jev, the filter, is the one key you must have.)

## Order

Providers are taken in this order, skipping any that are cooling down (see `cooldowns`):

1. A `provider` set in config, if any.
2. Providers with a key, in the order exa, parallel, sonar, youcom.
3. Keyless Exa, Parallel, You.com (their hosted MCP endpoints).
4. DuckDuckGo (HTML scrape).
5. SearXNG, when `searxng_url` is set.

`sources` (default 3) providers are queried at once. A provider that errors or answers empty is replaced by the next; fewer are used if fewer exist. Lists are fused by reciprocal rank (k=60) and each result carries the engines that returned it. `--sources 1` restores a plain fallback chain: first provider that answers. Timeouts: 12s per provider attempt, 30s per search.

## Backends

| name | keyless | key setting | notes |
|---|---|---|---|
| `exa` | yes: hosted MCP, about 50 searches per day per IP | `exa` (`EXA_API_KEY`) | neural index; best on research and code queries; returns page excerpts. Keyed: $20 intro credit, $10/month free, $7 per 1,000 |
| `parallel` | yes: hosted MCP, unpublished daily limit (a few dozen) | `parallel` (`PARALLEL_API_KEY`) | fast (~0.7s), page excerpts; keyed $1 per 1,000 |
| `youcom` | yes: free profile, unpublished limit (~70/day observed) | `youcom` (`YOUCOM_API_KEY`) | keyword results, short snippets; keyed $5 per 1,000, $100 signup credit |
| `sonar` | no | `sonar` (`SONAR_API_KEY`) | Perplexity; runs an LLM, 90s timeout |
| `ddg` | yes: HTML endpoint, unofficial, soft-blocks around 30/min per IP | none | short snippets; a cookie jar and 202 retry are built in |
| `searxng` | your own instance | `searxng` (`SEARXNG_URL`) | no quota; results depend on the engines it aggregates; see `searxng` |

Keyed use of a provider promotes it to the front of the chain and lifts the keyless caps. `keys validate` makes one lightweight call per configured provider.

## What Jev sees

Each result reaches Jev as title, URL, and a snippet of up to 600 characters. When a provider returns page text, the snippet starts at the first line that reads like prose, skipping navigation and bylines. The full excerpt is kept as `content` and stands in for a page that cannot be scraped.
