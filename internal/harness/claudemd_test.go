package harness

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestParseImports(t *testing.T) {
	content := `# My rules
@docs/style.md
See @~/.claude/shared.md for more.
Contact me at user@example.com — not an import.
Mention of @channel is not one either.
` + "```bash\n@ignored/inside-fence.md\n```\n" + `@last.md`

	got := parseImports([]byte(content))
	want := []string{"docs/style.md", "~/.claude/shared.md", "last.md"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("parseImports = %v, want %v", got, want)
	}
}

func TestClaudeMDBrokenAndNestedImports(t *testing.T) {
	dir := t.TempDir()
	write := func(name, content string) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
		return path
	}
	// root imports a (exists, which imports root back → cycle) and b (missing).
	root := write("CLAUDE.md", "@a.md\n@b.md\n")
	write("a.md", "@CLAUDE.md\n")

	c := &collector{look: fakeLookPath()}
	c.auditClaudeMD("user", root)

	var broken int
	for _, f := range c.findings {
		if strings.Contains(f.Issue, "does not resolve") {
			broken++
			if !strings.Contains(f.Issue, "b.md") {
				t.Errorf("wrong broken import: %+v", f)
			}
		}
	}
	if broken != 1 {
		t.Errorf("broken imports = %d, want 1; findings %+v", broken, c.findings)
	}
	// Inventory: CLAUDE.md + a.md, each once despite the cycle.
	if len(c.items) != 2 {
		t.Errorf("items = %d, want 2 (%+v)", len(c.items), c.items)
	}
}

func TestAnalyzeMemory(t *testing.T) {
	dir := t.TempDir()
	index := `# Memory Index
- [Kept](kept.md) — a healthy entry
- [Gone](missing.md) — file was deleted
- [Kept again](kept.md) — duplicate line
`
	for name, content := range map[string]string{
		"MEMORY.md": index,
		"kept.md":   "fact",
		"orphan.md": "never indexed",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}

	findings := analyzeMemory(dir, "user")
	var missing, dupe, orphan int
	for _, f := range findings {
		switch {
		case strings.Contains(f.Issue, "doesn't exist"):
			missing++
			if f.Severity != SeverityWarning {
				t.Errorf("missing entry should be a warning: %+v", f)
			}
		case strings.Contains(f.Issue, "times"):
			dupe++
		case strings.Contains(f.Issue, "not listed in MEMORY.md"):
			orphan++
			if filepath.Base(f.File) != "orphan.md" {
				t.Errorf("wrong orphan: %+v", f)
			}
		}
	}
	if missing != 1 || dupe != 1 || orphan != 1 {
		t.Errorf("missing=%d dupe=%d orphan=%d, want 1 each; %+v", missing, dupe, orphan, findings)
	}
}
