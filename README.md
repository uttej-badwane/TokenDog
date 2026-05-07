# TokenDog

```
████████╗ ██████╗ ██╗  ██╗███████╗███╗   ██╗██████╗  ██████╗  ██████╗
╚══██╔══╝██╔═══██╗██║ ██╔╝██╔════╝████╗  ██║██╔══██╗██╔═══██╗██╔════╝
   ██║   ██║   ██║█████╔╝ █████╗  ██╔██╗ ██║██║  ██║██║   ██║██║  ███╗
   ██║   ██║   ██║██╔═██╗ ██╔══╝  ██║╚██╗██║██║  ██║██║   ██║██║   ██║
   ██║   ╚██████╔╝██║  ██╗███████╗██║ ╚████║██████╔╝╚██████╔╝╚██████╔╝
   ╚═╝    ╚═════╝ ╚═╝  ╚═╝╚══════╝╚═╝  ╚═══╝╚═════╝  ╚═════╝  ╚═════╝
```

**A local HTTPS proxy that filters tool output before it reaches your AI assistant's context window.**

[![Release](https://img.shields.io/github/v/release/uttej-badwane/TokenDog)](https://github.com/uttej-badwane/TokenDog/releases)
[![License](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

TokenDog runs as a local HTTPS proxy between your AI assistant (Claude Code, Cursor, Cline, anything respecting `HTTPS_PROXY`) and `api.anthropic.com`. It intercepts every request the assistant sends, finds the `tool_result` content blocks the model is about to be charged for, and applies tool-specific compression — losslessly, before any tokens are billed.

```
$ td gain --since 1d
TokenDog Savings (last 24h)
══════════════════════════════════════════════════════════
Total commands:     142 (proxy: 138, hook: 4)
Saved:              48.2KB (12,403 tokens, 18.7%)
Cost saved:         $0.19 (per-model rates, cl100k)
```

## Install

```bash
brew tap uttej-badwane/tokendog
brew install tokendog
td setup
```

That's it. `td setup` handles every step:

1. Generates and trusts a local CA cert (TouchID prompt on macOS)
2. Installs a launchd LaunchAgent so the proxy auto-starts at login
3. Appends `HTTPS_PROXY=http://127.0.0.1:8888` to your shell rc
4. Removes any old `td hook claude` PreToolUse entry from `~/.claude/settings.json`
5. Verifies end-to-end with a synthetic Anthropic round-trip

After it finishes, **restart Claude Code** so it picks up the new env var.

To preview without changes: `td setup --dry-run`. To reverse: `td unsetup`.

### Other install paths

```bash
# Without brew
curl -fsSL https://raw.githubusercontent.com/uttej-badwane/TokenDog/main/scripts/install.sh | sh

# Docker
docker pull ghcr.io/uttej-badwane/tokendog:latest
```

Linux/Windows: `td setup` works for everything except cert install + launchd, which are macOS-only today (the command prints platform-specific manual steps for those).

## How it works

```
Claude Code (or any AI client respecting HTTPS_PROXY)
                    │
                    ▼  HTTPS_PROXY=http://127.0.0.1:8888
            ┌──────────────────┐
            │ TokenDog proxy   │   localhost daemon, MITMs api.anthropic.com only
            │  - parse Messages│
            │    API request   │
            │  - filter        │   Per-tool compaction: git status, gh api, kubectl,
            │    tool_result[] │   terraform plan, find, ls, jq, curl, ~25 in total
            │  - re-serialize  │
            └────────┬─────────┘
                     │ filtered payload
                     ▼
              api.anthropic.com
```

The proxy MITMs **only** `api.anthropic.com:443` — every other host's CONNECT is tunneled through unchanged. Trust footprint stays small.

Cache safety: only the **last** `tool_result` in the request is filtered. Anthropic's prompt cache hashes content; modifying historical content would invalidate the cache and net cost would go *up*. The last message contains content not yet seen by the API, so filtering it is a pure win.

## What gets filtered

| Tool | Strategy | Real-world reduction |
|---|---|---|
| `git status/log/diff/branch` | Compact format, drop `index abc..def` metadata | 30-85% |
| `gh pr/issue/run list` | Column-padding normalization, JSON compaction on `gh api` | 30-60% |
| `gh run view --log` | Strip per-line `job\tstep\ttimestamp` prefix repetition | 40-60% |
| `aws/gcloud/az` | Lossless JSON re-marshal, table normalization | 30-80% |
| `kubectl get/describe/top` | Table compaction, blank-line collapse | 20-60% |
| `terraform/tofu plan/apply` | Drop refresh + apply-progress spam, preserve resource diffs verbatim | 40-70% |
| `ls -la` | Drop permissions, owner, timestamps | 55-70% |
| `find` | Group paths by directory, skip `.git` / `node_modules` | 70-95% |
| `grep -rn` | Group matches by file path, dedupe path strings | 30-50% |
| `pytest/jest/vitest/go test/cargo test` | Collapse to summary on all-pass; verbatim on any failure | 60-95% |
| `npm/pnpm/yarn/pip` | Drop fetch/progress noise | 40-80% |
| `jq, curl` (JSON) | Lossless compaction, no indentation | 40-70% |
| `docker ps/images` | Compact tables | 20-40% |
| `make` | Drop successful-compile lines, keep warnings/errors verbatim | 30-70% |

**Lossless principle**: TokenDog never silently drops content. It restructures and removes structural noise. If filtering would lose data, the original passes through unchanged. Every filter has the universal `Guard` invariant: output bytes ≤ input bytes.

## Honest savings expectations

- Tool output (the part TD touches) is typically **30-50% of your Anthropic bill**.
- Per-tool reduction is 30-90% on the bytes TD compresses.
- Net bill reduction in proxy mode for a typical user: **5-15%** depending on how tool-output-heavy the workflow is.
- Run `td replay` against your own transcripts to get your specific number for your actual workflow.

## Three commands worth knowing

### `td gain` — your savings, accurately priced

```bash
td gain                    # all-time totals, per-model rates
td gain --since 7d         # last week
td gain --by-model         # opus / sonnet / haiku split
td gain --by-project       # cross-repo breakdown (.git-rooted detection)
td gain --daily            # day-by-day time series
td gain --json             # pipeable to jq or dashboards
```

### `td replay` — counterfactual: "what if I'd had td running all year?"

```bash
td replay                  # walk every transcript at ~/.claude/projects/
td replay --since 30d      # last 30 days
td replay --json           # machine-readable
```

Reads your historical Claude transcripts, replays each Bash tool_result through current filters, and shows what TD would have saved. Surfaces the top unhandled binaries (your priority list for new filter contributions).

### `td proxy` — the proxy lifecycle

```bash
td proxy daemon status       # is the launchd agent running?
td proxy daemon install      # (re)install the LaunchAgent
td proxy daemon uninstall    # stop and remove the agent
td proxy install-cert        # (re)install the CA cert
td proxy start               # run in foreground (Ctrl-C to stop)
```

Most users only run these via `td setup` — they're here for when something breaks or you want to inspect state.

## Privacy

The proxy sees every byte of every Anthropic API request — including conversation content, tool outputs, and any pasted secrets. Nothing leaves your machine; analytics writes to `~/.config/tokendog/` only. See [SECURITY.md](SECURITY.md) for the full data flow and threat model.

The `redact` package scrubs AWS keys, GitHub tokens, Slack tokens, JWTs, and PEM blocks from `td purge --redact` and `td replay --redact` output. The proxy itself does not redact in-flight content (the model needs the originals to do its job).

## MCP integration (Claude Desktop)

```bash
td mcp install     # adds tokendog to claude_desktop_config.json
td mcp doctor      # diagnoses Claude Desktop wiring
```

Exposes 5 tools to Claude Desktop so you can ask "how much has TokenDog saved me this week?" in chat. See [td mcp](#mcp-integration-claude-desktop) for the per-tool list.

## Architecture

```
.
├── cmd/                       cobra subcommands
├── internal/
│   ├── analytics/             history.jsonl + per-model aggregation
│   ├── cache/                 30s output cache for repeated commands (hook mode)
│   ├── filter/                ~25 per-tool compactors + universal Guard
│   ├── hook/                  PreToolUse rewrite logic + bash chain parsing
│   ├── mcpconfig/             Claude Desktop config management
│   ├── pricing/               embedded Anthropic model pricing
│   ├── proxy/                 HTTPS proxy + cert + launchd
│   ├── redact/                secret-scrubbing regex pack
│   ├── replay/                transcript walker + counterfactual savings
│   ├── tokenizer/             cl100k via tiktoken-go (Anthropic proxy ~10%)
│   └── transcript/            Claude session JSONL parser
└── scripts/install.sh         brew-less installer
```

Filter dispatch is a single registry (`internal/filter/registrations.go`). Adding a filter is one file + one line in registrations + one entry in `hook.Supported`. See [CONTRIBUTING.md](CONTRIBUTING.md).

## Status

Active. Recent changes in [CHANGELOG.md](CHANGELOG.md).

Looking for help with: more filters (`cat`, `helm`, `psql`, `dig` would all be useful), Linux launchd-equivalent (systemd user units), Windows scheduled-task auto-install. See [CONTRIBUTING.md](CONTRIBUTING.md).

## License

MIT
