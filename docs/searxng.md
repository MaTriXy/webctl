# Local SearXNG

The keyless tiers meter by the day. A SearXNG instance you run has no quota and aggregates Google, Bing, and others.

```
docker run -d --name searxng -p 8899:8080 \
  -v "$PWD/docs/searxng/settings.yml:/etc/searxng/settings.yml:ro" searxng/searxng:latest
webctl keys set searxng --value http://localhost:8899   # or: export SEARXNG_URL=http://localhost:8899
webctl config set provider searxng                      # optional: try it first
```

`docs/searxng/settings.yml` enables the JSON format the client needs and disables SearXNG's own rate limiter for local use. Results are keyword-grade with short snippets, so Jev has less to judge than with Exa's excerpts; `--scrape` closes that gap when it matters. SearXNG itself has no quota, but the engines behind it do: after a few hundred queries in a day Google CSE and Brave answer "too many requests" and SearXNG suspends them for a while, returning no results. Treat it as a supplement, not an unlimited source.
