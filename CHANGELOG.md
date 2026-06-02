# Changelog

All notable changes to TokenDog. Format follows [Keep a Changelog](https://keepachangelog.com/);
versions follow [Semantic Versioning](https://semver.org/).

## [Unreleased]

## [0.11.0] - 2026-06-02

### Added
- **Opt-in prose route (reversible-gated).** When a prose compressor is wired up (`TD_PROSE_ENDPOINT` → a localhost sidecar), the engine uses it to build a denser reversible preview for **natural-language** content instead of crude head/tail truncation. It runs **only inside the reversible pass** (so the full original is always stashed and recoverable via `td_retrieve` — lossy on the wire, never lossy in effect) and **only on content that looks like prose** (`looksLikeProse` rejects JSON/logs/code, where TokenDog's lossless filters win). The engine stays I/O-free: it calls an injected `core.ProseFunc`; the HTTP-to-sidecar client lives in `internal/prose` and the proxy/gateway inject it. Off by default; the client times out (`TD_PROSE_TIMEOUT_MS`, default 2s) and falls back to the head/tail preview on any error. A reference sidecar + protocol is in `experiments/prose-sidecar/`.

### Changed
- **Architecture: engine decoupled from the MITM proxy.** The compression logic moved into a provider-neutral `internal/core` package with a clean `Compress(conversation) → savings` API. Wire-format handling lives in pluggable adapters (`internal/adapter/anthropic`, `internal/adapter/openai`) behind a `core.Provider` interface + `core.Dispatch(path, body)` router. The HTTPS proxy is now a thin frontend over this engine — one deployment of several, not *the* product. This is the foundation for gateway/SDK deployments, multi-provider support, and an offline eval harness, none of which should require a CA cert or Anthropic-only assumptions. Existing proxy behavior is unchanged (same tests pass).

### Added
- **Multi-provider USD pricing.** `internal/pricing` now carries OpenAI (gpt-4o family, o-series), Amazon Bedrock (Claude-hosted + Nova), and Gemini rate tables alongside Anthropic's. Records are priced with a provider-aware resolver (`pricing.LookupFor` / `ProviderDefault`): an exact resolved model wins, otherwise a non-Anthropic record prices at that provider's default model instead of being mis-priced as Anthropic Opus. `td gain --by-provider` now shows a **USD saved** column per provider. `pricing.Lookup` was hardened to prefer the longest matching prefix so `gpt-4o-mini-…` no longer collides with `gpt-4o`. Legacy Anthropic records are unchanged.
- **Per-provider tokenization + `td gain --by-provider`.** Token counts in analytics now use the right tiktoken encoding for the provider a record came from — `o200k_base` for OpenAI (gpt-4o family), `cl100k_base` for Anthropic / Bedrock-Claude / Gemini — instead of always using the cl100k Anthropic proxy. The gateway and proxy tag each savings record with its provider (`core.Dispatch` now returns the matched provider name), and the tokenizer loads/caches encoders per encoding. New `td gain --by-provider` shows a per-provider savings breakdown; legacy untagged records fold into "anthropic (proxy)". `analytics.NewRecordForProvider` is the provider-aware record constructor (`NewRecord` stays as the Anthropic-default wrapper).
- **`td fleet` — fleet observability + centrally-managed policy.** Turns the per-laptop tool into something a platform team can run across an org. `td fleet push --endpoint URL` reports aggregate savings (commands, bytes, tokens + a hashed machine id) to an internal collector — opt-in and privacy-preserving: the payload carries **no** command strings, arguments, or tool output. `td fleet pull <url>` installs a managed policy (`internal/policy`) that governs the engine's dedup / reversible toggles and the reversible-stash threshold; `td fleet policy` shows the effective config. Precedence is env override > managed policy > built-in default, so a platform baseline never traps a developer who needs to deviate. The engine reads policy via `core.OptionsFromEnv`.
- **`td eval` — offline quality harness.** Proves compression is quality-neutral with numbers instead of vibes, no live model needed. Each corpus fixture declares the answer-bearing facts a task would need (`must_keep`); the harness compresses it through the real engine and checks every fact survives. Reports two measures per fixture: **inline** (fact in the prompt the model receives — no retrieval) and **recoverable** (reachable at all — inline, via the reversible stash, or via a dedup back-reference). PASSES only if every fact is recoverable, so it doubles as a CI gate (`exit 1` on any lost fact) and as a regression test that a per-tool filter never strips an answer. Ships an embedded starter corpus (lossless filter, generic JSON, dedup, reversible); `--corpus DIR` runs your own, `--json` for machines. See `internal/eval/`.
- **`td gateway` — explicit-base_url reverse proxy (no CA cert).** A security-team-friendly alternative to the MITM proxy: point your SDK's base URL at `http://127.0.0.1:8099` and the gateway compresses tool output and forwards to the real provider over normal TLS. No root-CA install, no trust-store changes, no interception of traffic you didn't redirect. `--port` / `--upstream` flags. Routes Anthropic (`/v1/messages`) and OpenAI (`/v1/chat/completions`) by path through the shared engine.
- **OpenAI Chat Completions adapter** (`internal/adapter/openai`): the same dedup / per-tool filter / generic / reversible passes now run on OpenAI-shaped requests (tool results as `role:"tool"` messages, commands from `tool_calls[].function`). Proves the engine is genuinely provider-neutral.
- **Amazon Bedrock Converse adapter** (`internal/adapter/bedrock`): the gateway now compresses Bedrock traffic too — point `td gateway --upstream https://bedrock-runtime.<region>.amazonaws.com` and the engine handles the nested `toolResult`/`toolUse` content-block shape, preserving all Bedrock-specific fields (status, inferenceConfig, …). The "Bedrock middleware" deployment with no SDK change beyond the endpoint.
- **`td learn`**: closes the loop on reversible compression. Every `td_retrieve` call is now logged to `~/.config/tokendog/retrievals.jsonl`; `td learn` joins that against the stash events in analytics and reports a per-command retrieve rate. A high rate means the head/tail preview is too aggressive for that command's output shape — the model keeps pulling the full original back. `--json` and `--top N` flags. New retrieval telemetry lives in `internal/stash/telemetry.go`.
- **Generic JSON fallback filter**: output from commands with no per-tool filter is now sniffed by shape, not binary name — a single JSON value (object or array) is re-marshalled without indentation. Catches the long tail of unhandled commands (`curl`/`httpie`, custom `--output json` CLIs). Lossless and runs only when no per-tool filter claimed the output. See `internal/filter/generic.go`.
- **Reversible compression** (opt-in via `TD_REVERSIBLE=1`): the proxy stashes the full original of any large tool output under `~/.config/tokendog/originals/` and injects a compact head/tail preview carrying a `[td:STASHED id=…]` marker. The model recovers the original on demand through the new `td_retrieve` MCP tool. This is the first path that goes beyond the lossless ceiling — nothing is lost, only deferred to an on-demand round-trip — and it covers the long tail of commands that have no per-tool filter. New `td stash list/get/purge` subcommands inspect the store. Tunable with `TD_STASH_MIN` (min bytes, default 2048) and `TD_STASH_TTL` (retention seconds, default 24h).
- **Cross-message dedup**: when the last message's `tool_result` is byte-identical to one from an earlier message in the same request, the proxy replaces it with a compact back-reference (`[td: identical to the output of … N tool outputs earlier …]`) instead of re-billing the full copy. Lossless (the original is verbatim above, in the model's own context) and cache-safe (touches only the last message). Works for any tool output — including re-reads via the Read tool, which have no per-tool filter. On by default; `TD_NO_DEDUP=1` to disable.
- **Filter registry**: every binary→filter mapping now lives in `internal/filter/registrations.go`. Adding a new filter touches one file instead of five (filter source, cmd wrapper, root registration, replay dispatch, hook Supported map).
- **history.jsonl rotation**: archive at 100k records or 90 days to `history-YYYY-MM.jsonl.gz`. Prevents `td gain` from getting slower over time.
- **Hot-path benchmarks** (`internal/hook/hook_bench_test.go`): `BenchmarkProcessClaudeSimple`, `Chain`, `BashC`, `Unsupported`, `SplitChain`, `ParseBinary`. Sub-microsecond budget for the per-Bash-call hot path.
- **Filter golden-file tests**: coverage on `internal/filter` lifted from 44.9% to 74.7%. New tests for cloud, jq, curl, kubectl, make, pkg, and test runners.

### Removed
- **`internal/calibration` package** (dead code since v0.7.0). Its multiplier was measuring session-token-density rather than tokenizer accuracy and overstated USD by 100x+ in real workloads. Per-model pricing via `internal/pricing` replaces it.

### Fixed
- Cache tests now clear `TD_NO_CACHE` env var before running so a developer who has it set globally doesn't see false-negative test failures.

## [0.7.0] - 2026-05-06

### Added
- **Per-model pricing**: `internal/pricing` package with embedded snapshot of Anthropic rates (Opus 4.7, 4.6, Sonnet 4.6, 4.5, Haiku 4.5, plus Claude 3.x). Versioned model ids resolve via prefix match.
- **`Record.Model` field**: populated lazily at gain/replay time by reading the session's transcript and taking PredominantModel.
- **`td gain --by-model`**: per-model breakdown showing calls, tokens saved, and USD at each model's actual rate.
- **`td gain --daily` / `--monthly`**: time-series breakdown by calendar period. Composes with `--by-model` and date filters.
- **`td gain --since` / `--until`**: filter records before aggregation. Accepts ISO (`2026-04-01`), compact (`20260401`), or relative (`7d`/`2w`/`1m`/`1y`).
- **`td gain --json` and `td replay --json`**: machine-readable output for piping into dashboards or other tools.

### Changed
- **Headline cost line in `td gain`** now sums per-model USD using each model's actual rate. Records with no resolved model land in an "unknown" bucket priced at DefaultModel as a conservative upper bound; the imputed-percentage is shown so users can judge precision.

### Removed
- **Calibrator interface**: the multiplier was misleading (overstated USD 100x+). Removed from `RenderGain`. The calibration package itself stays for one more release as a diagnostic.

## [0.6.0] - 2026-05-06

### Added
- **`td replay`**: walks `~/.claude/projects/**/*.jsonl`, replays every historical Bash tool_result through current filters, projects counterfactual savings. Reports per-command, per-session, and unhandled-binary breakdowns. `--days`, `--max-sessions`, `--top`, `--price-per-million` flags.
- **`hook.ParseBinary`**: extract underlying binary + args from a Claude-emitted command string. Handles bash -c unwrap, env-var prefixes, path-prefixed binaries. Shared between live hook rewrite and replay classification.

### Fixed
- **`filter.Ls` panic** on plain `ls` output (no `-l` flag). Lines with 9+ space-delimited filenames tripped the field-count heuristic and panicked at `perms[3:]`. Now validates fields[0] looks like a 10-char permission string before treating as long-format.

## [0.5.0] - 2026-05-06

### Added
- **Chain operator parsing**: `cd /path && supported-cmd` now rewrites correctly. Splitter handles `&&`, `||`, `;` outside quoted regions; bails out conservatively on backticks, `$(...)`, heredocs, and escaped operators.
- **Hook env-var injection**: each rewritten command now carries `TD_SESSION_ID` and `TD_TRANSCRIPT_PATH` so analytics can attribute records to a Claude session. Uses `export` form when chains are detected so vars propagate across all segments.
- **Per-session view**: `td gain --session=<id>` (or `--session=current`) shows TD savings alongside Anthropic's reported session totals from the transcript.
- **`internal/transcript`**: streaming-aware JSONL reader following ccstatusline's dedup rule (count finalized rows + the latest unfinished one).

### Fixed
- Pre-existing latent bug: `TD_SESSION_ID=foo cd /path && td git status` scoped the var to only the `cd` segment. Export form fixes propagation.

## [0.4.5] and earlier

See git log. Pre-changelog releases.
