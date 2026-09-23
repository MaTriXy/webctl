# Contributing

Thanks for looking at webctl. Bug reports, fixes, benchmark results, and
docs corrections are all welcome. Open an issue first for anything larger
than a small fix so we can agree on the shape before you write it.

## Search providers

webctl supports many search backends, and every one of them was added
because someone who uses webctl wanted it in their chain. That is the bar
for new ones too.

**Provider PRs are user-driven, not vendor-driven.** If you work for,
are paid by, or otherwise have a stake in the provider you are adding,
please do not open a PR for it. Open an issue instead that says who you
are, links the API docs, and explains what the provider does that the
existing ones do not. If a webctl user then wants it, they can pick it up,
or ask us to.

This is not a judgement about any vendor. A provider PR from the vendor
is, in effect, sponsored content: the description, the placement in the
chain, and the pricing notes are written by the party that benefits from
them, and the maintainers end up doing marketing review instead of code
review. Adapters written by users describe what the provider is actually
like to use, which is what the docs are for.

If you would like webctl users to be able to use your API today, you can
ship the adapter yourself. `internal/provider` is small and the pattern is
easy to copy; a fork or a wrapper in your own repo is a fine home for it,
and we are happy to link to it from an issue.

**Always disclose affiliations.** Whatever you are contributing, say in
the PR if you have a commercial relationship with anything it touches.
Disclosed conflicts are easy to handle; discovered ones are not.

## Code

- `go vet ./... && go test ./...` must pass; CI runs exactly that.
- `gofmt` your changes.
- Keep providers to the existing shape: a `Search`, a `Validate`, and a
  test that hits a stub server. See `internal/provider/tavily.go`.
- Docs live in `docs/`; update `docs/providers.md` and `docs/config.md`
  when a provider or key changes.
