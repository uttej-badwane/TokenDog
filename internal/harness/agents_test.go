package harness

import (
	"strings"
	"testing"
)

func TestParseFrontmatter(t *testing.T) {
	fields, present, err := parseFrontmatter([]byte("---\nname: reviewer\ndescription: \"Reviews PRs for style violations\"\ntools: Read, Grep\n---\nBody text\n"))
	if err != nil || !present {
		t.Fatalf("present=%v err=%v", present, err)
	}
	if fields["name"] != "reviewer" || fields["tools"] != "Read, Grep" {
		t.Errorf("fields = %v", fields)
	}
	if fields["description"] != "Reviews PRs for style violations" {
		t.Errorf("quotes not trimmed: %q", fields["description"])
	}

	if _, present, _ := parseFrontmatter([]byte("# Just markdown\n")); present {
		t.Error("plain markdown should have no frontmatter")
	}
	if _, present, err := parseFrontmatter([]byte("---\nname: x\nno closing fence\n")); !present || err == nil {
		t.Error("unterminated frontmatter should error")
	}
}

func TestAnalyzeAgentFile(t *testing.T) {
	// Healthy agent: no findings.
	good := []byte("---\nname: deploy-checker\ndescription: Validates deployment manifests before every release push\ntools: Read, Bash\n---\n")
	parses, _, findings := analyzeAgentFile("/a.md", "user", good)
	if !parses || len(findings) != 0 {
		t.Errorf("healthy agent: parses=%v findings=%+v", parses, findings)
	}

	// Missing description + tools.
	_, _, findings = analyzeAgentFile("/a.md", "user", []byte("---\nname: x\n---\n"))
	var missingDesc, missingTools bool
	for _, f := range findings {
		if strings.Contains(f.Issue, `"description"`) {
			missingDesc = true
		}
		if strings.Contains(f.Issue, "inherits every tool") {
			missingTools = true
		}
	}
	if !missingDesc || !missingTools {
		t.Errorf("missingDesc=%v missingTools=%v: %+v", missingDesc, missingTools, findings)
	}

	// No frontmatter at all.
	parses, parseErr, findings := analyzeAgentFile("/a.md", "user", []byte("just prose"))
	if parses || parseErr == "" || len(findings) != 1 {
		t.Errorf("no frontmatter: parses=%v err=%q findings=%+v", parses, parseErr, findings)
	}
}

func TestAnalyzeSkillFile(t *testing.T) {
	good := []byte("---\nname: deploy\ndescription: Runs the staged deploy checklist for this repo\n---\n")
	parses, _, findings := analyzeSkillFile("/SKILL.md", "user", good)
	if !parses || len(findings) != 0 {
		t.Errorf("healthy skill: parses=%v findings=%+v", parses, findings)
	}
	_, _, findings = analyzeSkillFile("/SKILL.md", "user", []byte("---\n---\n"))
	if len(findings) != 2 {
		t.Errorf("empty frontmatter should flag name+description, got %+v", findings)
	}
}

func TestAnalyzeKeybindings(t *testing.T) {
	parses, _, findings := analyzeKeybindings("/k.json", []byte(`{
		"bindings": [
			{"key": "ctrl+s", "command": "save"},
			{"key": "ctrl+s", "command": "submit"},
			{"key": "ctrl+r", "context": "chat", "command": "retry"}
		]
	}`))
	if !parses {
		t.Fatal("valid JSON should parse")
	}
	if len(findings) != 1 || !strings.Contains(findings[0].Issue, `"ctrl+s"`) {
		t.Errorf("duplicate binding not flagged: %+v", findings)
	}

	parses, parseErr, findings := analyzeKeybindings("/k.json", []byte("{nope"))
	if parses || parseErr == "" || len(findings) != 1 || findings[0].Severity != SeverityWarning {
		t.Errorf("invalid JSON: parses=%v err=%q findings=%+v", parses, parseErr, findings)
	}
}

func TestAnalyzeSettingsMap(t *testing.T) {
	cfg := map[string]any{
		"model":        "opus",
		"permisions":   map[string]any{}, // classic typo
		"allowedTools": []any{"Bash"},    // deprecated
	}
	findings := analyzeSettingsMap("/s.json", "user", cfg)
	var typo, deprecated bool
	for _, f := range findings {
		if strings.Contains(f.Issue, `"permisions"`) && f.Severity == SeverityInfo {
			typo = true
		}
		if strings.Contains(f.Issue, `"allowedTools"`) && f.Severity == SeverityWarning {
			deprecated = true
		}
	}
	if !typo || !deprecated || len(findings) != 2 {
		t.Errorf("typo=%v deprecated=%v findings=%+v", typo, deprecated, findings)
	}
}
