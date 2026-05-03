# TokenDog

```
████████╗ ██████╗ ██╗  ██╗███████╗███╗   ██╗██████╗  ██████╗  ██████╗
╚══██╔══╝██╔═══██╗██║ ██╔╝██╔════╝████╗  ██║██╔══██╗██╔═══██╗██╔════╝
   ██║   ██║   ██║█████╔╝ █████╗  ██╔██╗ ██║██║  ██║██║   ██║██║  ███╗
   ██║   ██║   ██║██╔═██╗ ██╔══╝  ██║╚██╗██║██║  ██║██║   ██║██║   ██║
   ██║   ╚██████╔╝██║  ██╗███████╗██║ ╚████║██████╔╝╚██████╔╝╚██████╔╝
   ╚═╝    ╚═════╝ ╚═╝  ╚═╝╚══════╝╚═╝  ╚═══╝╚═════╝  ╚═════╝  ╚═════╝
```

**Token-optimized CLI proxy for AI coding assistants.** TokenDog sits between your AI assistant (Claude Code, Cursor, etc.) and your shell, compressing command output and tool responses before they reach the model — saving 60–90% of tokens on common dev operations.

[![Release](https://img.shields.io/github/v/release/uttej-badwane/TokenDog)](https://github.com/uttej-badwane/TokenDog/releases)
[![License](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

---

## Why

Claude Code and similar tools run hundreds of `git`, `ls`, `find`, and `grep` commands per session, fetch GitHub pages, search the web, and pipe huge JSON through `jq` — each emitting verbose output that gets read into context. A single `git log` block, a 600KB GitHub page, or a `find` that hits `node_modules` can burn thousands of tokens for content the model rarely needs.

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
    ],
    "PostToolUse": [
      { "matcher": "WebFetch",   "hooks": [{ "type": "command", "command": "td pipe webfetch" }] },
      { "matcher": "Glob",       "hooks": [{ "type": "command", "command": "td pipe glob" }] },
      { "matcher": "Grep",       "hooks": [{ "type": "command", "command": "td pipe grep" }] },
      { "matcher": "WebSearch",  "hooks": [{ "type": "command", "command": "td pipe websearch" }] }
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

### Tool response interception (PostToolUse hook)

| Tool | Strategy | Typical savings |
|------|----------|-----------------|
| `WebFetch` | Strip HTML structural noise (script, style, nav, header, footer) | **40–99%** |
| `Glob` | Group paths by directory | **40–70%** |
| `Grep` | Group matches by file with line numbers | **30–50%** |
| `WebSearch` | Collapse redundant whitespace | **10–25%** |

**Lossless principle:** TokenDog never silently drops content. It restructures and removes structural noise. If filtering would lose data, the original is passed through unchanged.

---

## Usage

### Hook-based (automatic)

Once configured, TokenDog rewrites Claude Code's Bash calls and intercepts tool responses transparently. You don't run anything manually — Claude calls `git status`, the hook rewrites it to `td git status`, and the filtered output goes back to Claude.

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
┌─────────────────────────────────────────────────────┐
│              Claude Code / AI Assistant              │
└────────────┬─────────────────────────┬──────────────┘
             │ PreToolUse: Bash        │ PostToolUse: WebFetch/Glob/Grep/WebSearch
             ▼                         ▼
      ┌──────────────┐         ┌──────────────────┐
      │ td hook      │         │ td pipe ...      │
      │ claude       │         │                  │
      │              │         │ ─ extract result │
      │ ─ rewrite    │         │ ─ strip noise    │
      │   command to │         │ ─ collapse ws    │
      │   td <sub>   │         │ ─ return JSON    │
      └──────┬───────┘         └────────┬─────────┘
             │                          │
             ▼                          ▼
      Original cmd runs              Modified response
      via td <sub>                   sent back to model
      with filter applied
```

- **PreToolUse** rewrites Bash commands so they execute through TokenDog's filters.
- **PostToolUse** intercepts tool results (WebFetch, Grep, Glob, WebSearch) and returns a compressed version.
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

Numbers based on observed usage of ~1.5M tokens saved per active dev per day. WebFetch is by far the largest contributor for sessions involving GitHub repo analysis (a single repo browse can save 600KB+ per page).

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

# Hook handlers (used by Claude Code, not invoked manually)
td hook claude           Process PreToolUse hooks (stdin → stdout JSON)
td pipe webfetch         Process PostToolUse for WebFetch
td pipe glob             Process PostToolUse for Glob
td pipe grep             Process PostToolUse for Grep
td pipe websearch        Process PostToolUse for WebSearch
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
