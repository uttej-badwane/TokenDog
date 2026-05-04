# TokenDog

```
████████╗ ██████╗ ██╗  ██╗███████╗███╗   ██╗██████╗  ██████╗  ██████╗
╚══██╔══╝██╔═══██╗██║ ██╔╝██╔════╝████╗  ██║██╔══██╗██╔═══██╗██╔════╝
   ██║   ██║   ██║█████╔╝ █████╗  ██╔██╗ ██║██║  ██║██║   ██║██║  ███╗
   ██║   ██║   ██║██╔═██╗ ██╔══╝  ██║╚██╗██║██║  ██║██║   ██║██║   ██║
   ██║   ╚██████╔╝██║  ██╗███████╗██║ ╚████║██████╔╝╚██████╔╝╚██████╔╝
   ╚═╝    ╚═════╝ ╚═╝  ╚═╝╚══════╝╚═╝  ╚═══╝╚═════╝  ╚═════╝  ╚═════╝
```

**Token-optimized CLI proxy for AI coding assistants.** TokenDog sits between your AI assistant (Claude Code, Cursor, etc.) and your shell, compressing command output before it reaches the model — saving 60–90% of tokens on common dev operations.

[![Release](https://img.shields.io/github/v/release/uttej-badwane/TokenDog)](https://github.com/uttej-badwane/TokenDog/releases)
[![License](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

---

## Why

Claude Code and similar tools run hundreds of `git`, `ls`, `find`, and `grep` commands per session and pipe huge JSON through `jq` — each emitting verbose output that gets read into context. A single `git log` block, a noisy `kubectl get`, or a `find` that hits `node_modules` can burn thousands of tokens for content the model rarely needs.

TokenDog filters this output **losslessly** — it strips structural noise (HTML tags, hint lines, permission bits, redundant whitespace) without dropping a single piece of meaningful content. The model still gets every fact it needs, just without the boilerplate.

---

## Install

```bash
brew tap uttej-badwane/tokendog
brew install tokendog
```

Verify and see setup status:
```bash
td welcome     # colored welcome screen — auto-detects what's configured
```

Both `td` and `tokendog` work as commands (symlinked).

---

## Setup with Claude Code

Add the following to `~/.claude/settings.json`:

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

That's it. Run `td welcome` again — every checkmark should turn green.

---

## What It Filters

### Bash command rewrites (PreToolUse hook)

| Command | Strategy | Typical savings |
|---------|----------|-----------------|
| `git status` | Strip hints, restructure to one line per state | **70–85%** |
| `git log` | Compact format, full commit body preserved | **30–60%** |
| `git diff` | Drop `index abc..def` metadata | **15–25%** |
| `git branch` | One branch per line, current marked | **10–20%** |
| `ls -la` | Drop permissions, owner, timestamps | **55–70%** |
| `find` | Group paths by directory, skip `.git` / `node_modules` | **70–95%** |
| `docker ps` / `images` | Compact table | **20–40%** |
| `jq` | Lossless JSON compaction (no indentation) | **40–70%** |
| `curl` | JSON-aware response compression — values preserved | **40–80%** |
| `kubectl get` / `top` / `describe` | Table compression, blank-line collapse | **20–60%** |
| `gh pr/issue/run/repo list` | Column-padding normalization, bodies untouched | **30–60%** |
| `pytest` / `jest` / `vitest` / `go test` / `cargo test` | Collapse to summary on all-pass; verbatim on any failure | **60–95%** |
| `npm` / `pnpm` / `yarn` / `pip` | Drop fetch/progress noise, keep warnings & errors | **40–80%** |
| `aws` / `gcloud` / `az` | Lossless JSON compaction, table normalization, YAML blank-line collapse | **30–80%** |
| `make` | Drop compile spam, keep warnings/errors verbatim | **30–70%** |

**Lossless principle:** TokenDog never silently drops content. It restructures and removes structural noise. If filtering would lose data, the original is passed through unchanged.

> **Why no PostToolUse hooks?** Claude Code's PostToolUse hook can inject `additionalContext` but cannot replace the `tool_response` already sent to the model. That makes it impossible to compact native tools (`Glob`, `Grep`, `WebFetch`, `WebSearch`) after the fact — earlier versions exposed `td pipe *` for this, but it was a no-op against current Claude Code, so it has been removed.

---

## Usage

### Hook-based (automatic)

Once configured, TokenDog rewrites Claude Code's Bash calls transparently. You don't run anything manually — Claude calls `git status`, the hook rewrites it to `td git status`, and the filtered output goes back to Claude.

### Manual (CLI)

You can also invoke filters directly:

```bash
td git status              # filtered git status
td git log -10             # last 10 commits, compact, full body preserved
td ls                      # clean ls
td find . -name "*.go"     # grouped find
td docker ps               # compact docker
td jq '.items[].name'      # compact jq output
td curl https://api.example.com/data
td kubectl get pods
td kubectl describe deploy myapp
td gh pr list                 # compact gh tables
td pytest tests/              # summary on all-pass, verbatim on failure
td go test ./...              # same strict-mode for go test
td npm install                # drops fetch/progress lines
td aws ec2 describe-instances # lossless JSON compaction
td make                       # drops successful-compile lines
```

### Analytics

```bash
td gain                    # summary of savings
td gain --history          # recent commands with per-call savings
```

Sample output:

```
TokenDog Savings
════════════════════════════════════════════════════════════
Total commands:        29
Raw output:            1.9MB
After filter:          31.6KB
Saved:                 1.9MB (~486660 tokens, 98.4%)
Efficiency:            ███████████████████████░ 98.4%
```

### Discover missed savings

`td discover` scans your Claude Code session history and ranks every Bash command you've run, showing which ones went through TokenDog and which ones bypassed it.

```bash
td discover
```

```
Scanned 29 session files, 196 Bash commands
  Already through td:   142 (72.4%)
  Direct (not via td):  54

Top commands (missed = ran directly without td)
──────────────────────────────────────────────────────────────────────────
  Command                 Total    Missed   Coverage   Status
──────────────────────────────────────────────────────────────────────────
  export                  71       71          0.0%   not handled
  gh                      9        9           0.0%   not handled
  git                     21       0         100.0%   ✓ fully covered
  ls                      12       0         100.0%   ✓ fully covered
  ...
```

Use this to identify which filters to install hooks for, or to request new ones via GitHub Issues.

### Debug

```bash
td rewrite "git log --oneline -20"   # show how the hook would rewrite
```

---

## How It Works

```
┌─────────────────────────────────────────┐
│       Claude Code / AI Assistant         │
└────────────────┬────────────────────────┘
                 │ PreToolUse: Bash
                 ▼
          ┌──────────────┐
          │ td hook      │
          │ claude       │
          │              │
          │ ─ rewrite    │
          │   command to │
          │   td <sub>   │
          └──────┬───────┘
                 │
                 ▼
        Original cmd runs
        via td <sub>
        with filter applied
```

- **PreToolUse / Bash** rewrites the Bash `command` field so the rewritten version executes through TokenDog's filters. The hook returns `hookSpecificOutput.updatedInput` per Claude Code's current schema.
- All commands record analytics in `~/.config/tokendog/history.jsonl`.

---

## Cost Savings

At current Claude API pricing:

| Scale | Tokens saved/month | Sonnet 4.6 saved | Opus 4.7 saved |
|-------|--------------------|-------------------|-----------------|
| 1 dev | ~30M | $90 | $450 |
| 10 devs | ~300M | $900 | $4,500 |
| 50 devs | ~1.5B | $4,500 | $22,500 |
| 100 devs | ~3B | $9,000 | $45,000 |

Numbers based on observed usage of Bash filters (`gh`, `aws`, `git`, `kubectl`, `find`, `jq`, package managers, test runners). Heavy `gh` and cloud-CLI workflows benefit the most.

---

## Commands

```
td welcome               Colored welcome screen with setup status
td discover              Find unrewritten commands in your Claude history
td gain                  Show savings summary
td gain --history        Savings + recent command history
td rewrite <cmd>         Debug: show how a command would be rewritten

# Bash filters (auto-invoked via hook)
td git <subcmd>          git with compact output
td ls [args]             List files, structured
td find [args]           find with grouped output
td docker <subcmd>       docker with compact tables
td jq [args]             jq with compact JSON output
td curl [args]           curl with JSON-aware response compression
td kubectl <subcmd>      kubectl get/describe/top with compact output
td gh <subcmd>           gh with column padding normalized
td pytest [args]         pytest with all-pass summary collapse
td jest [args]           jest with all-pass summary collapse
td vitest [args]         vitest with all-pass summary collapse
td go test [args]        go test with PASS-line collapse (other go subcmds pass through)
td cargo <subcmd>        cargo test/build/check with progress stripped
td npm [args]            npm with fetch/progress noise stripped
td pnpm [args]           pnpm with fetch/progress noise stripped
td yarn [args]           yarn with fetch/progress noise stripped
td pip [args]            pip with download/progress noise stripped
td aws [args]            aws CLI with JSON/table compaction (lossless)
td gcloud [args]         gcloud CLI with JSON/YAML/table compaction (lossless)
td az [args]             az CLI with JSON/table compaction (lossless)
td make [args]           make with successful-compile lines dropped

# Hook handler (used by Claude Code, not invoked manually)
td hook claude           Process PreToolUse hooks (stdin → stdout JSON)
```

---

## Roadmap

- [ ] Custom `.tokendog.toml` filter files (per-project user-defined rules)
- [ ] Cloud sync for team-wide analytics dashboard
- [ ] Cursor / Cline / Aider hook integrations
- [ ] Additional filters: `gh`, `terraform`, `npm`, `cargo`, `pytest`, `jest`
- [ ] LLM-assisted summarization for unknown command output

---

## Contributing

Issues and PRs welcome. The architecture is intentionally simple:

- `cmd/` — CLI commands (cobra-based)
- `internal/filter/` — per-tool filter implementations (one file per tool)
- `internal/hook/` — hook protocol parsers
- `internal/welcome/` — first-run welcome experience
- `internal/analytics/` — local savings tracking

Build and test locally:
```bash
go build -o td .
go test ./...
```

To add a new filter:
1. Create `internal/filter/<tool>.go` with a function that takes raw output and returns filtered output
2. Create `cmd/<tool>.go` with a cobra command that runs the tool, captures stdout, calls the filter
3. Add the tool name to the `supported` map in `internal/hook/hook.go`
4. Register the cobra command in `cmd/root.go`

---

## License

MIT — see [LICENSE](LICENSE).
