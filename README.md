# TokenDog

```
████████╗ ██████╗ ██╗  ██╗███████╗███╗   ██╗██████╗  ██████╗  ██████╗
╚══██╔══╝██╔═══██╗██║ ██╔╝██╔════╝████╗  ██║██╔══██╗██╔═══██╗██╔════╝
   ██║   ██║   ██║█████╔╝ █████╗  ██╔██╗ ██║██║  ██║██║   ██║██║  ███╗
   ██║   ██║   ██║██╔═██╗ ██╔══╝  ██║╚██╗██║██║  ██║██║   ██║██║   ██║
   ██║   ╚██████╔╝██║  ██╗███████╗██║ ╚████║██████╔╝╚██████╔╝╚██████╔╝
   ╚═╝    ╚═════╝ ╚═╝  ╚═╝╚══════╝╚═╝  ╚═══╝╚═════╝  ╚═════╝  ╚═════╝
```

**A CLI proxy that filters Bash output before it reaches your AI assistant's context window.**

[![Release](https://img.shields.io/github/v/release/uttej-badwane/TokenDog)](https://github.com/uttej-badwane/TokenDog/releases)
[![License](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

Claude Code (or Cursor, Cline, Aider) runs hundreds of `git`, `ls`, `find`, `gh`, `kubectl`, `aws` commands per session. Each one's stdout becomes input tokens on the next turn. TokenDog sits in the hook path, applies tool-specific compression, and reduces what your assistant has to ingest — losslessly, before any tokens are charged.

```
$ td replay
TokenDog Hindsight
Replayed:                  107 sessions, 3,572 Bash calls (1,743 handled)
Output volume:             1.9MB raw → 1.6MB filtered
Would-have-saved:          95,620 tokens (16.1%)
Projected cost saved:      $1.43 at $15/M (Opus 4.7 standard)
```

## Install

```bash
brew tap uttej-badwane/tokendog && brew install tokendog
```

Or grab a binary directly:
```bash
curl -fsSL https://raw.githubusercontent.com/uttej-badwane/TokenDog/main/scripts/install.sh | sh
```

Or pull the Docker image:
```bash
docker pull ghcr.io/uttej-badwane/tokendog:latest
```

## Set up the hook

Add to `~/.claude/settings.json`:

```json
{
  "hooks": {
    "PreToolUse": [
      {
        "matcher": "Bash",
        "hooks": [{ "type": "command", "command": "td hook claude" }]
      }
    ]
  }
}
```

That's it. Run `td welcome` to verify everything is wired up.

## What's filtered

| Tool | Strategy | Real-world savings |
|---|---|---|
| `git status/log/diff/branch` | Strip hints, compact format, drop `index abc..def` metadata | 30–85% |
| `gh pr/issue/run list` | Column-padding normalization (bodies untouched) | 30–60% |
| `gh api`, `aws/gcloud/az` | Lossless JSON re-marshal | 30–80% |
| `kubectl get/describe/top` | Table compression, blank-line collapse | 20–60% |
| `ls -la` | Drop permissions, owner, timestamps | 55–70% |
| `find` | Group paths by directory, skip `.git` / `node_modules` | 70–95% |
| `pytest` / `jest` / `vitest` / `go test` / `cargo test` | Collapse to summary on all-pass; verbatim on any failure | 60–95% |
| `npm` / `pnpm` / `yarn` / `pip` / `cargo build` | Drop fetch/progress noise | 40–80% |
| `jq`, `curl` (JSON) | Lossless compaction, no indentation | 40–70% |
| `docker ps/images` | Compact tables | 20–40% |
| `make` | Drop successful-compile lines, keep warnings/errors | 30–70% |

**Lossless principle**: TokenDog never silently drops content. It restructures and removes structural noise. If filtering would lose data, the original passes through unchanged. Every filter has the `Guard` invariant: output bytes ≤ input bytes.

## Three commands worth knowing

### `td gain` — your savings, accurately priced

```bash
td gain                    # all-time totals, with calibrated USD per model
td gain --by-model         # opus vs haiku vs sonnet split
td gain --daily            # day-by-day breakdown
td gain --since 7d         # last week
td gain --json             # pipeable to jq, dashboards, ccusage
td gain --session=current  # this Claude session only
```

Per-model pricing (`internal/pricing`) tracks Opus 4.7, Sonnet 4.6, Haiku 4.5, plus older models, with input/output/cache rates. The headline cost line uses each session's actual model — no hardcoded $15/M assumption.

### `td replay` — counterfactual: "what if I'd had td running all year?"

```bash
td replay              # walk every transcript at ~/.claude/projects/
td replay --days 30    # last 30 days
td replay --json       # machine-readable
```

Reads your historical Claude transcripts, replays each Bash tool_result through current filters, and tells you what TD would have saved. Also surfaces the top unhandled binaries (your priority list for new filter contributions).

### `td discover` — coverage audit

```bash
td discover    # which Bash commands in your history bypassed td
```

Catches misconfigured hooks. If `gh: 70 calls, 0% coverage` shows up, your hook isn't matching.

## How does this compare to ccusage?

[ccusage](https://github.com/ryoppippi/ccusage) tells you what you spent. TokenDog tells you how much less you would have spent if your tool output had been filtered. They're complementary — run both.

`td gain --json` and `td replay --json` are designed to flow into ccusage-style dashboards.

## Cost math

The README of older versions claimed "$450/dev/month at Opus pricing." That number was extrapolated from a heavy-workload sample and didn't reflect typical usage. The honest answer:

> Run `td replay` against your own transcripts. Whatever number it shows is your actual potential savings on the data you've already generated. Most users will see $1–$30/month; heavy `aws describe-*` / `kubectl get -o json` / `gh run view --log` workloads see 5–10×.

Going forward, `td gain` accumulates real numbers as you use the tool, and per-model pricing makes the dollar figure trustworthy.

## Architecture

```
~/.claude/settings.json
        │ Bash command
        ▼
┌──────────────────┐
│ td hook claude   │   PreToolUse hook
│  - splitChain    │   (chain operators, bash -c, env vars)
│  - rewrite       │
│  - inject session│
└────────┬─────────┘
         │ rewritten command
         ▼
┌──────────────────┐
│ td <tool> <args> │
│  - cache check   │   30s TTL, env-aware
│  - exec wrapped  │
│  - filter.Apply  │   registry → tool-specific filter
│  - Guard         │   lossless invariant
│  - analytics     │   per-record token counts via cl100k
│  - cache write   │
└──────────────────┘
```

Filter dispatch is a single registry (`internal/filter/registrations.go`). Adding a filter is one source file + one line in registrations + one entry in `hook.Supported`. See [CONTRIBUTING.md](CONTRIBUTING.md).

## Privacy

`td replay` reads your historical Claude transcripts, which may contain pasted secrets. TokenDog runs entirely offline (no telemetry, no network) and writes only to `~/.config/tokendog/`. See [SECURITY.md](SECURITY.md) for the full data flow and threat model.

## Status

Active. Recent changes in [CHANGELOG.md](CHANGELOG.md).

Looking for help with: more filters (`grep`, `cat`, `terraform`, `psql`, `helm` would all be useful), MCP server for Claude Desktop integration, Linux package repos. See [CONTRIBUTING.md](CONTRIBUTING.md).

## License

MIT
