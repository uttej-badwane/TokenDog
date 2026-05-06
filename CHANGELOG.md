# Changelog

All notable changes to TokenDog. Format follows [Keep a Changelog](https://keepachangelog.com/);
versions follow [Semantic Versioning](https://semver.org/).

## [Unreleased]

### Added
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
