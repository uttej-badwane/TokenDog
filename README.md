# TokenDog

```
              .---""""---.
            .'   /\   /\  '.
           /    /  \-/  \   \
          |    |  o   o  |   |
          |     \   ▼   /    |
          |      '.___.'     |
           \    /|     |\   /
            '. / |\ / /| \.'
              \  | V V |  /
               '-+-----+-'
                T O K E N D O G
```

**Token-optimized CLI proxy for AI coding assistants.** TokenDog sits between your AI assistant (Claude Code, Cursor, etc.) and your shell, compressing command output and tool responses before they reach the model — saving 60–90% of tokens on common dev operations.

[![Release](https://img.shields.io/github/v/release/uttej-badwane/TokenDog)](https://github.com/uttej-badwane/TokenDog/releases)
[![License](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

---

## Why

Claude Code and similar tools run hundreds of `git`, `ls`, `find`, and `grep` commands per session, fetch GitHub pages, and search the web — each emitting verbose output that gets read into context. A single `git log` block, a 600KB GitHub page, or a `find` that hits `node_modules` can burn thousands of tokens for content the model rarely needs.

TokenDog filters this output **losslessly** — it strips structural noise (HTML tags, hint lines, permission bits, redundant whitespace) without dropping a single piece of meaningful content. The model still gets every fact it needs, just without the boilerplate.

---

## Install

```bash
brew tap uttej-badwane/tokendog
brew install tokendog
```

Verify:
```bash
td --version
tokendog --version    # both work — symlinked
```

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

That's it. Every relevant tool call is now intercepted and compressed automatically.

---

## What It Filters

| Tool / Command | Strategy | Typical savings |
|----------------|----------|-----------------|
| `Bash` → `git status` | Strip hints, restructure to one line per state | **70–85%** |
| `Bash` → `git log` | Compact format, full commit body preserved | **30–60%** |
| `Bash` → `git diff` | Drop `index abc..def` metadata | **15–25%** |
| `Bash` → `ls` | Drop permissions, owner, timestamps | **55–70%** |
| `Bash` → `find` | Group paths by directory, skip `.git`/`node_modules` | **70–95%** |
| `Bash` → `docker ps`/`images` | Compact table | **20–40%** |
| `WebFetch` | Strip HTML structural noise (script, style, nav) | **40–99%** |
| `Glob` | Group paths by directory | **40–70%** |
| `Grep` | Group matches by file | **30–50%** |
| `WebSearch` | Collapse redundant whitespace | **10–25%** |

**Lossless principle:** TokenDog never drops content. It restructures and removes noise. If filtering would lose data, it leaves the original untouched.

---

## Usage

### Hook-based (automatic)

Once configured, TokenDog rewrites Claude Code's Bash calls and intercepts tool responses transparently. You don't run anything manually — Claude calls `git status`, the hook rewrites it to `td git status`, and the filtered output goes back to Claude.

### Manual (CLI)

You can also invoke filters directly:

```bash
td git status              # filtered git status
td git log -10             # last 10 commits, compact
td ls                      # clean ls
td find . -name "*.go"     # grouped find
td docker ps               # compact docker
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
             │ PreToolUse: Bash        │ PostToolUse: WebFetch/Glob/Grep
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

Numbers based on observed usage of ~1.5M tokens saved per active dev per day. WebFetch is by far the largest contributor for sessions involving GitHub repo analysis.

---

## Commands

```
td git <subcmd>          Git with compact output
td ls [args]             List files, structured
td find [args]           Find with grouped output
td docker <subcmd>       Docker with compact tables
td hook claude           Process Claude Code PreToolUse hooks (stdin → stdout JSON)
td pipe webfetch         Process Claude Code PostToolUse for WebFetch
td pipe glob             Process Claude Code PostToolUse for Glob
td pipe grep             Process Claude Code PostToolUse for Grep
td pipe websearch        Process Claude Code PostToolUse for WebSearch
td gain                  Show savings summary
td gain --history        Savings + recent commands
td rewrite <cmd>         Debug: show how a command would be rewritten
```

---

## Roadmap

- [ ] Cloud sync for team-wide analytics dashboard
- [ ] Custom `.tokendog.toml` filter files (per-project)
- [ ] LLM-assisted summarization for unknown command output
- [ ] Cursor / Cline / Aider hook integrations
- [ ] Filters: `kubectl`, `npm`, `cargo`, `gh`, `pytest`

---

## Contributing

Issues and PRs welcome. The architecture is intentionally simple:
- `cmd/` — CLI commands (cobra-based)
- `internal/filter/` — per-tool filter implementations
- `internal/hook/` — hook protocol parsers
- `internal/analytics/` — local savings tracking

Build:
```bash
go build -o td .
go test ./...
```

---

## License

MIT — see [LICENSE](LICENSE).
