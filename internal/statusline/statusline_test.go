package statusline

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRenderPlain(t *testing.T) {
	t.Setenv("NO_COLOR", "1") // deterministic, escape-free output

	got := Render(Payload{
		Dir:        "/tmp/does-not-exist-xyz", // no .git ⇒ no branch segment
		Model:      "Opus 4.8",
		Effort:     "high",
		ContextPct: 8,
		CostUSD:    2.1,
	})
	want := "does-not-exist-xyz  Opus 4.8 high  8% ctx  $2.10"
	if got != want {
		t.Errorf("Render = %q, want %q", got, want)
	}
}

func TestRenderOmitsContextWhenZero(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	got := Render(Payload{Dir: "/tmp/none-here-xyz", Model: "Haiku", CostUSD: 0.01})
	if strings.Contains(got, "ctx") {
		t.Errorf("expected no context segment at 0%%, got %q", got)
	}
	if !strings.HasSuffix(got, "$0.01") {
		t.Errorf("expected cost suffix, got %q", got)
	}
}

func TestRenderColorized(t *testing.T) {
	t.Setenv("NO_COLOR", "") // note: present-but-empty still disables per LookupEnv
	os.Unsetenv("NO_COLOR")
	got := Render(Payload{Dir: "/tmp/none-here-xyz", Model: "Opus", CostUSD: 1})
	if !strings.Contains(got, "\x1b[") {
		t.Errorf("expected ANSI color codes, got %q", got)
	}
}

func TestGitBranchReadsHEAD(t *testing.T) {
	repo := t.TempDir()
	gitDir := filepath.Join(repo, ".git")
	if err := os.MkdirAll(gitDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte("ref: refs/heads/feature/x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// From a nested subdir, gitBranch should walk up to the repo root.
	sub := filepath.Join(repo, "a", "b")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if got := gitBranch(sub); got != "feature/x" {
		t.Errorf("gitBranch = %q, want feature/x", got)
	}
}

func TestGitBranchDetachedHEAD(t *testing.T) {
	repo := t.TempDir()
	gitDir := filepath.Join(repo, ".git")
	_ = os.MkdirAll(gitDir, 0o755)
	_ = os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte("0123456789abcdef\n"), 0o644)
	if got := gitBranch(repo); got != "0123456" {
		t.Errorf("detached gitBranch = %q, want short sha 0123456", got)
	}
}

func TestCtxColorThresholds(t *testing.T) {
	if ctxColor(10, false) != green || ctxColor(60, false) != yellow || ctxColor(90, false) != red {
		t.Error("context color thresholds wrong")
	}
	if ctxColor(5, true) != red {
		t.Error("exceeds_200k should force red")
	}
}
