# Security policy

## Reporting a vulnerability

Email the maintainer privately: open a [GitHub Security Advisory](https://github.com/uttej-badwane/TokenDog/security/advisories/new) (preferred) or a private email if that path isn't available.

Please include:
- A short description of the issue
- A reproduction (sample command, transcript snippet, or hook config)
- The output of `td --version`

We aim to acknowledge within 72 hours and ship a fix within 14 days for high-severity issues.

Please do not file public issues for vulnerabilities.

## What TokenDog reads and writes

TokenDog is a CLI tool that wraps other CLI tools. It is invoked by your AI assistant's hook system on every Bash tool call. You should know what it touches.

### Reads
- **Stdout of the wrapped command** — whatever `git`/`gh`/`aws`/etc. emits. TD only filters the output; it does not modify the command's behavior or arguments.
- **`~/.claude/projects/**/*.jsonl`** when you run `td replay` or `td gain --session`. These transcripts contain the full content of your Claude conversations, which may include pasted secrets (API keys, `.env` contents, AWS credentials, private code). TD reads raw bytes and runs them through the same filters that would have run live.
- **Sensitive env vars listed in `internal/cache/cache.go:envVarsThatAffectOutput`** — specifically AWS_*, GCP/CLOUDSDK_*, AZURE_*, KUBECONFIG, GITHUB_TOKEN, GH_TOKEN, GIT_DIR, GIT_WORK_TREE. These contribute to the cache key so a different profile gets a different cache slot. The values are **hashed**, not stored in plaintext.

### Writes
- **`~/.config/tokendog/history.jsonl`** — one record per command. Stores the command line, byte/token counts, duration, and (for v0.5.0+) session_id and transcript_path. Does **not** store the command's stdout content.
- **`~/.config/tokendog/cache/<hash>.json`** — short-TTL cache (30s default) of filtered output. Contents are the **filtered** stdout, not raw. Pruned automatically.
- **Stdout** — the filtered output, which is what your AI assistant sees.

### Does not write
- Network. TD is fully offline. The tiktoken vocab is downloaded once (on first invocation) by the `pkoukk/tiktoken-go` library and cached locally; after that, no network.
- Other directories. TD never writes outside `~/.config/tokendog/` and the file paths it directly produces (e.g. archive files in the same dir).

## Known threat model

### What TD protects against
- A misbehaving filter producing more bytes than its input (`Guard()` enforces ≤ raw size).
- Command injection via session env vars: `injectSessionEnv` validates `TD_SESSION_ID` matches `[a-zA-Z0-9_-]+` and rejects `TD_TRANSCRIPT_PATH` containing `'`/`\n`/`\\`/`\0`.
- Filter dispatch picking up the wrong tool when commands are wrapped in `bash -c` or chained: chain parsing bails out on shell constructs we can't reliably parse (`$(...)`, backticks, heredocs).

### What TD does NOT protect against
- An attacker with shell access on your machine. TD has no privilege boundary; if someone can run `td`, they can read your history.jsonl.
- Malicious filters. The filter package is part of TD; trust it as you trust TD itself. Third-party filters (when TD adds plugin support) will need a separate trust review.
- Secrets pasted into Claude conversations and replayed by `td replay`. We read the transcripts as-is. If you ran `td replay --json | upload-somewhere`, you'd be uploading those transcripts. There is currently no `--no-content` redaction mode (planned for a future release).

## Recommendations for production / regulated environments

- Set `TOKENDOG_DISABLE=1` in environments where output filtering must not happen.
- Periodically `rm ~/.config/tokendog/history.jsonl` to limit retention. TD will rotate at 100k records / 90 days but that's a soft cap.
- Treat `~/.config/tokendog/` as containing PII-equivalent data and back up / delete accordingly.
- Pin TD to a known-good version in your build pipeline (`brew install tokendog@0.7.0` or equivalent) rather than tracking `latest`.

## Past advisories

None to date.
