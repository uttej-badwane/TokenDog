# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What TokenDog is

TokenDog (`td`) is a provider-neutral compression engine that sits between an AI coding agent and the model, finds the `tool_result` blocks in each request, and losslessly shrinks them **before the tokens are billed**. It ships as a single Go binary. The core contract is that compression is **cache-safe and never silently lossy**: output bytes ≤ input bytes for every filter, and if compression would lose signal the original passes through unchanged.

## Common commands

```bash
go build -o td .                 # builds in <1s; binary is `td`
go test ./...                    # full suite, ~5s
go test -race -count=1 ./...     # what CI runs
go test ./internal/filter/       # one package
go test -run TestGitStatus ./internal/filter/   # one test
go vet ./...
gofmt -l .                       # must be empty — CI fails on unformatted files
gofmt -w .                       # fix formatting

# hot-path latency (run before/after any internal/hook change; post numbers in PR)
go test -bench=BenchmarkProcessClaude -run NONE -benchtime=1s ./internal/hook/
```

Debug the hook rewriter without a running proxy: `td rewrite "<command>"` shows how a Bash command would be rewritten. `td gain` shows savings analytics.

CI (`.github/workflows/test.yml`) runs gofmt, `go vet`, race tests, `golangci-lint` (config in `.golangci.yml` — `errcheck` is disabled), a build matrix across ubuntu/macos/windows, and report-only `govulncheck`. All of gofmt / vet / test must pass.

## Architecture: engine + adapters + frontends

The design deliberately separates three concerns so each extends independently:

- **`internal/core`** — the engine. `core.Compress(*Conversation) → []Saving` runs over a provider-neutral `Conversation`. It knows nothing about HTTP, analytics, or any vendor. `core.Dispatch` routes a request body to the first registered `Provider` whose `Match(path)` accepts it. This is the reusable, testable heart — most logic changes belong here or in `internal/filter`.
- **`internal/adapter/{anthropic,openai,bedrock}`** — each translates one wire format (Anthropic Messages, OpenAI Chat Completions, Bedrock Converse) into a `Conversation` and writes replacements back into the original payload. Adapters self-register via `init()` calling `core.Register`. **Adding a provider is one new adapter; the engine is untouched.**
- **frontends** — supply transport + the analytics sink. `td proxy` (MITM, needs a local CA cert) and `td gateway` (explicit `base_url`, no cert) are two frontends over the same `core.Dispatch`. `internal/proxy` is a thin frontend, not where compression lives.

### The compression pipeline (`internal/core/compress.go`)

For each eligible `tool_result`, `Compress` tries in order and takes the first win: **dedup** (identical earlier output → one-line back-reference) → **per-tool filter** → **generic JSON compaction** → **reversible stash** (opt-in via `TD_REVERSIBLE=1`; stashes the full original locally and sends a head/tail preview + marker the model can pull back via the `td_retrieve` MCP tool).

### The filter registry (`internal/filter`)

Each per-tool filter is one file implementing `func(args []string, raw string) string`, self-registering via `init()` → `Register("git", Git)`. `internal/filter` has **no external dependencies** by design. `filter.Apply` wraps every filter in the universal `Guard`, which enforces output ≤ input. The registry is the single source of truth consumed by both live compression and `td replay`.

Adding a filter (the most common contribution — see CONTRIBUTING.md) touches exactly three places: the new `internal/filter/<tool>.go` file, one `Register(...)` line in `internal/filter/registrations.go`, and one entry in the `Supported` map in `internal/hook/hook.go`.

### The hook is on the hot path (`internal/hook`)

`td hook claude` runs as a Claude Code PreToolUse hook **before every Bash tool call**, rewriting e.g. `git status` → `td git status` (sub-microsecond budget). `internal/hook` is the shell-aware layer: `RewriteCommand` / `splitChain` / `unwrapShellC` / `injectSessionEnv` parse and rewrite command strings. This parsing is security-sensitive — it is a command-injection surface. It deliberately **bails out unchanged** (safety over coverage) on `$(...)`, backticks, and heredocs, which need a real shell parser. Do not add coverage for those without one. `ParseBinary` mirrors the same rules for `td replay` so a command classifies identically live and on replay.

### Lossless contract (see CONTRIBUTING.md)

Every filter MUST produce output ≤ input bytes, preserving every meaningful byte ("meaningful" excludes whitespace runs, padding, and structural noise like git-diff `index abc..def` headers). When in doubt, pass through unchanged — a no-op filter is a success; one that corrupts or reorders data is a bug. Prefer golden-file tests over length assertions, and include an adversarial-input passthrough test so a filter never panics.

## Modules and non-Go components

The root Go module is `tokendog` (Go 1.23; `main.go` just calls `cmd.Execute()`, all subcommands live in `cmd/`). Two components are **separate modules/toolchains**, not part of `go test ./...`:

- `tray/` — Windows/Linux system-tray app, its own Go module (uses cgo, intentionally isolated).
- `macos/TokenDogBar/` — native Swift menu-bar app; reads `td spend` / `td harness --json`.

## Style

- `gofmt -w .` before committing. Comment the **why**, not the **what**.
- Respect package boundaries: `internal/filter` has no external deps, `cmd/` is the integration layer, `internal/hook` is shell-aware.
- Follow the PR flow in CONTRIBUTING.md: include before/after `td replay` output when adding a filter, and hook benchmark numbers when touching `internal/hook`.
