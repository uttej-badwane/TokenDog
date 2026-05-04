package filter

import (
	"strings"
	"testing"
)

// TestGitStatusErrorPassthrough is the regression test for v0.4.4's
// "gitStatus returns clean for any unrecognized output" bug. An error
// like `fatal: not a git repository` must be passed through verbatim,
// never replaced with "clean\n".
func TestGitStatusErrorPassthrough(t *testing.T) {
	in := "fatal: not a git repository (or any of the parent directories): .git\n"
	out := gitStatus(in)
	if out != in {
		t.Errorf("expected error passed through verbatim\n got: %q\nwant: %q", out, in)
	}
}

func TestGitStatusClean(t *testing.T) {
	in := `On branch main
Your branch is up to date with 'origin/main'.

nothing to commit, working tree clean
`
	out := gitStatus(in)
	// Compact form for clean repo includes only the branch line.
	if !strings.Contains(out, "branch:") || !strings.Contains(out, "main") {
		t.Errorf("expected branch line in clean status, got: %q", out)
	}
	if strings.Contains(out, "fatal:") {
		t.Errorf("error text should never appear in clean status: %q", out)
	}
}

func TestGitStatusModified(t *testing.T) {
	in := `On branch main
Changes not staged for commit:
  (use "git add <file>..." to update what will be committed)
	modified:   foo.go
	modified:   bar.go
`
	out := gitStatus(in)
	if !strings.Contains(out, "modified") {
		t.Errorf("expected 'modified' in output, got: %q", out)
	}
	if !strings.Contains(out, "foo.go") || !strings.Contains(out, "bar.go") {
		t.Errorf("expected file names preserved, got: %q", out)
	}
}

// TestGitLogOnelineNoTruncation is the regression test for v0.4.4's
// "git log --oneline truncated to 30" bug. The user explicitly specified
// the count via -N, so the filter must not silently drop commits.
func TestGitLogOnelineNoTruncation(t *testing.T) {
	var lines []string
	for i := 0; i < 50; i++ {
		lines = append(lines, "abc1234 commit message")
	}
	in := strings.Join(lines, "\n") + "\n"
	out := gitLog(in)
	gotLines := strings.Count(out, "commit message")
	if gotLines != 50 {
		t.Errorf("gitLog dropped commits: got %d, want 50", gotLines)
	}
	if strings.Contains(out, "more)") {
		t.Errorf("output should not contain truncation marker, got: %q", out)
	}
}

func TestGitDiffStripsIndex(t *testing.T) {
	in := `diff --git a/foo.go b/foo.go
index abcd1234..efgh5678 100644
--- a/foo.go
+++ b/foo.go
@@ -1,3 +1,3 @@
 unchanged
-removed
+added
`
	out := gitDiff(in)
	if strings.Contains(out, "index abcd1234..efgh5678") {
		t.Errorf("expected index line stripped, got: %q", out)
	}
	if !strings.Contains(out, "-removed") || !strings.Contains(out, "+added") {
		t.Errorf("hunk content must be preserved, got: %q", out)
	}
	if strings.Contains(out, "--- a/") || strings.Contains(out, "+++ b/") {
		t.Errorf("a/ b/ prefixes should be stripped, got: %q", out)
	}
}
