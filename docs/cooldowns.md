# Cooldowns

A provider that answers 429 (rate limited) or 402 (quota spent) is left alone for a growing window so that no search wastes a request on it and the block is not prolonged.

## Ladder

Consecutive failures climb `cooldown.steps`, default `15m, 1h, 4h, 12h, 24h, 72h`. A 402 starts at step `cooldown.quota_start` (default 3, i.e. 4h) because a spent quota does not return in fifteen minutes. Any successful call resets the provider to zero strikes.

At the top of the ladder the provider stays parked; one probe request is allowed every `cooldown.probe_interval` (default 24h). A failed probe re-arms the top window without escalating; a success clears it.

## State

`<config dir>/cooldown.json`, one entry per provider: when it was limited, the HTTP status, strike count, next retry time, last probe, last notice. Every process reads it, so a CLI search, an agent's search, and an eval run all honor the same windows. Keyed and keyless use of a provider are tracked separately (`exa` vs `exa+key`); setting a key clears that provider. Entries expired for more than a day are dropped.

## What you see

A skipped provider is announced on stderr once an hour per provider, every time with `--verbose`:

```
exa skipped: cooling down until Mon 14:20 (rate limited 12m ago, strike 2 of 6); retry now with `webctl cooldown clear exa`, adjust with `webctl config set cooldown.steps ...`
```

If nothing is available the search fails with exit 1 and lists every provider's state. Add a key, set `searxng_url`, or wait.

## Commands and settings

```
webctl cooldown                       # table: provider, strike, status, window
webctl cooldown clear [provider]      # forget one or all
webctl config set cooldown.steps 30m,2h,8h,24h,72h
webctl config set cooldown.probe_interval 12h
webctl config set cooldown.quota_start 4
webctl config set cooldown.enabled false
```

Environment: `WEBCTL_COOLDOWN_STEPS="30m,2h,8h"`, `WEBCTL_COOLDOWN_ENABLED=false`, and so on.
