# Contributing to TokenDog

Thanks for considering a contribution. This doc covers the things that aren't obvious from reading the code.

## Quick start

```bash
git clone git@github.com:uttej-badwane/TokenDog.git
cd TokenDog
go build -o td .
go test ./...
go test -bench=. -run NONE ./internal/hook/    # hot-path latency
```

`td` builds in <1s. The full test suite runs in ~5s.

## Adding a new filter

This is the most common contribution. Say you want to filter `grep` output.

**1. Implement the filter.** Create `internal/filter/grep.go`:

```go
package filter

import "strings"

// Grep compresses grep output by [your strategy].
// Lossless contract: output bytes ≤ input bytes; matched lines preserved.
func Grep(args []string, raw string) string {
    // ...
}
```

**2. Register it.** Add ONE line to `internal/filter/registrations.go`:

```go
func init() {
    // ... existing registrations
    Register("grep", Grep)
}
```

**3. Tell the hook to rewrite it.** Add `"grep"` to the `Supported` map in `internal/hook/hook.go`.

**4. Write tests.** Create `internal/filter/grep_test.go` with at least:
- A golden-file test using realistic real-world input
- A lossless-contract assertion (`len(out) <= len(in)`)
- A passthrough test for adversarial input (filter shouldn't panic)

**5. Run the suite.**

```bash
go test ./internal/filter/    # your new tests
go vet ./...                  # static analysis
gofmt -l .                    # formatting (must be empty)
```

That's it. You don't need to touch `cmd/`, `internal/replay/`, or any cobra registration. The registry handles the rest.

## The lossless contract

Every filter MUST produce output ≤ input in bytes. The universal `filter.Guard` at the wrapper layer enforces this, but filters should also pass through unchanged when they can't help. Inflating output is a contract violation.

A filter is lossless when:
- Every meaningful byte from the input is reachable in the output (possibly reformatted)
- "Meaningful" excludes whitespace runs, indentation, padding, and structural noise like `index abc..def` git-diff headers
- A reader of the output can reconstruct the model's understanding of what happened

Examples:
- ✅ `git log` filter dropping `Author: <email>` keeping just the name
- ✅ `aws ec2 describe-instances` JSON re-serialized without indent
- ❌ `find` filter dropping every 3rd path "to save space" — silently lossy
- ❌ `gh pr view` filter rewording the PR body — model gets misleading data

When in doubt: pass through unchanged. A filter that's a no-op is a successful filter; one that corrupts data is a bug.

## Architecture notes for non-trivial changes

### The hook is on the hot path
`td hook claude` runs **before every Bash tool call** Claude makes. Sub-microsecond budget. Run benchmarks before you commit:

```bash
go test -bench=BenchmarkProcessClaude -run NONE -benchtime=1s ./internal/hook/
```

Post the before/after numbers in your PR if you touch `internal/hook/`.

### Don't break analytics replay
`td replay` walks historical transcripts and runs current filters against them. Live and replay share a single dispatcher (`internal/filter/registry.go`), so anything that classifies a binary as "supported" must work for both.

### Be careful with shell-quoting
The hook rewrites Bash commands. Any change to command-string parsing (`splitChain`, `unwrapShellC`, `injectSessionEnv`) is a potential injection vector. We bail out on `$(...)`, backticks, heredocs because they need a real shell parser. **Don't add coverage for those without a real parser.**

## Style

- Run `gofmt -w .` before committing — CI fails on unformatted files.
- Follow existing package boundaries: `internal/filter` has no external deps, `cmd/` is the integration layer, `internal/hook` is shell-aware.
- Comment the **why**, not the **what**. If a comment just restates the code, delete it.
- Write tests that fail loudly when behavior changes — golden inputs > assertions about lengths.

## Pull request flow

1. Fork → branch → commit → push → open PR.
2. CI runs `go test`, `go vet`, `gofmt -l` on every push. All three must pass.
3. Brief PR description with: what changed, why, and a sample of before/after `td replay` output if you added a filter.
4. The maintainer will review within a few days. Drive-by review from other contributors is welcome.

## Reporting bugs

Use GitHub Issues. Include:
- `td --version`
- Output of `td rewrite "<your command>"` when the bug is about hook rewriting
- A minimal repro (smallest input that triggers the issue)

## Reporting security issues

See [SECURITY.md](./SECURITY.md). Please don't file public issues for vulnerabilities.

## License

MIT — see [LICENSE](./LICENSE). By contributing, you agree your changes are licensed the same.
