# Configuration

Settings resolve in this order: command-line flag, `MULTI_SEARCH_WEB_*` environment variable, `~/multi_search_web/config.yaml`, built-in default. API keys live in a separate file.

## Settings

| setting | env | default | meaning |
|---|---|---|---|
| `provider` | `MULTI_SEARCH_WEB_PROVIDER` | (auto) | provider to put first in the chain |
| `sources` | `MULTI_SEARCH_WEB_SOURCES` | 3 | providers queried and fused per search |
| `num` | `MULTI_SEARCH_WEB_NUM` | 10 | results requested per provider |
| `min_score` | `MULTI_SEARCH_WEB_MIN_SCORE` | 1.8 | relevance cut on the 0–3 rubric |
| `jev.base_url` | `MULTI_SEARCH_WEB_JEV_BASE_URL` | `https://api.typesafe.ai` | Jev API root |
| `jev.model` | `MULTI_SEARCH_WEB_JEV_MODEL` | `jev-latest` | Jev model |
| `searxng_url` | `SEARXNG_URL` | (none) | SearXNG instance; the keys file's `searxng_url` wins |
| `keys_file` | `MULTI_SEARCH_WEB_KEYS_FILE` | `~/secrets/keys.json` | where keys live |
| `cooldown.enabled` | `MULTI_SEARCH_WEB_COOLDOWN_ENABLED` | true | skip rate-limited providers |
| `cooldown.steps` | `MULTI_SEARCH_WEB_COOLDOWN_STEPS` | `15m,1h,4h,12h,24h,72h` | window per consecutive failure |
| `cooldown.probe_interval` | `MULTI_SEARCH_WEB_COOLDOWN_PROBE_INTERVAL` | `24h` | one probe this often at the top step |
| `cooldown.quota_start` | `MULTI_SEARCH_WEB_COOLDOWN_QUOTA_START` | 3 | step a 402 starts at |

`config.yaml` uses the same names, nested for dots:

```yaml
provider: parallel
sources: 2
min_score: 2.0
jev:
  model: jev-1.13.0
cooldown:
  steps: [30m, 2h, 8h, 24h, 72h]
```

## Commands

```
multi_search_web config show            # every setting, effective value, and source
multi_search_web config get sources
multi_search_web config set sources 2   # validated, written to config.yaml
multi_search_web config unset sources
multi_search_web config list            # settings and their meanings
multi_search_web config path
```

## Keys

`~/secrets/keys.json` (mode 0600), shared with other tools; fields this tool does not know are preserved. Fields: `exa_api_key`, `parallel_api_key`, `sonar_api_key`, `youcom_api_key`, `searxng_url`, `jev_api_key`. Environment overrides: `EXA_API_KEY`, `PARALLEL_API_KEY`, `SONAR_API_KEY`, `YOUCOM_API_KEY`, `SEARXNG_URL`, `JEV_API_KEY`.

```
multi_search_web setup                  # interactive wizard, validates each key
multi_search_web keys list              # what is set and where it came from
multi_search_web keys set exa           # masked prompt; --value for scripts
multi_search_web keys unset exa
multi_search_web keys validate          # one lightweight call per key
```

Setting a key clears that provider's cooldown. The Jev key is required; without it only `--no-filter` searches work. Search keys are optional.

## Files

| path | contents |
|---|---|
| `~/multi_search_web/config.yaml` | settings |
| `~/multi_search_web/cooldown.json` | provider cooldown state |
| `~/secrets/keys.json` | API keys |
| `~/multi_search_web/evals/*.json` | eval runs (one file each) |
