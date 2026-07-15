package harness

import (
	"strings"
	"testing"
)

const fakeAnthropicKey = "sk-ant-api03-aaaaaaaaaaaaaaaaaaaaaaaa"

func TestScanSecrets(t *testing.T) {
	content := []byte("line one is fine\nkey: " + fakeAnthropicKey + "\n")
	findings := scanSecrets("/s.json", "user", content)
	if len(findings) != 1 {
		t.Fatalf("findings = %+v, want 1", findings)
	}
	f := findings[0]
	if f.Severity != SeverityCritical {
		t.Errorf("severity = %s, want critical", f.Severity)
	}
	if !strings.Contains(f.Issue, "anthropic-api-key") || !strings.Contains(f.Issue, "line 2") {
		t.Errorf("issue should name the pattern and line: %q", f.Issue)
	}
	// The raw value must never leak into the report.
	if strings.Contains(f.Issue, fakeAnthropicKey) || strings.Contains(f.Fix, fakeAnthropicKey) {
		t.Errorf("finding echoes the secret: %+v", f)
	}
}

func TestScanSecretsPEMAndClean(t *testing.T) {
	pem := []byte("-----BEGIN RSA PRIVATE KEY-----\nMIIabc\n-----END RSA PRIVATE KEY-----\n")
	findings := scanSecrets("/s.json", "user", pem)
	if len(findings) != 1 || !strings.Contains(findings[0].Issue, "PEM") {
		t.Errorf("PEM block not flagged once: %+v", findings)
	}

	if got := scanSecrets("/s.json", "user", []byte(`{"model": "opus", "env": {}}`)); len(got) != 0 {
		t.Errorf("clean config should have no findings: %+v", got)
	}
	// Non-UTF8 content is skipped, not crashed on.
	if got := scanSecrets("/bin", "user", []byte{0xff, 0xfe, 0x00}); got != nil {
		t.Errorf("binary content should be skipped: %+v", got)
	}
}

func TestAnalyzeMCPServers(t *testing.T) {
	servers := map[string]any{
		"good": map[string]any{"command": "present-bin"},
		"gone": map[string]any{"command": "absent-bin"},
		"web":  map[string]any{"url": "https://mcp.example.com"},
		"bare": map[string]any{},
		"leaky": map[string]any{
			"command": "present-bin",
			"env":     map[string]any{"API_KEY": fakeAnthropicKey},
		},
	}
	findings := analyzeMCPServers("/.mcp.json", "project", servers, fakeLookPath("present-bin"))

	var missing, malformed, secret int
	for _, f := range findings {
		switch {
		case strings.Contains(f.Issue, "not found on PATH"):
			missing++
		case strings.Contains(f.Issue, "neither a command nor a url"):
			malformed++
		case strings.Contains(f.Issue, "inline secret"):
			secret++
			if f.Severity != SeverityCritical {
				t.Errorf("inline secret should be critical: %+v", f)
			}
			if strings.Contains(f.Issue, fakeAnthropicKey) {
				t.Errorf("finding echoes the secret: %+v", f)
			}
		}
	}
	if missing != 1 || malformed != 1 || secret != 1 {
		t.Errorf("missing=%d malformed=%d secret=%d, want 1 each; %+v", missing, malformed, secret, findings)
	}
}

func TestAnalyzeCrossScope(t *testing.T) {
	user := map[string]any{"model": "opus", "outputStyle": "explanatory", "permissions": map[string]any{}}
	proj := map[string]any{"model": "sonnet", "outputStyle": "explanatory", "permissions": map[string]any{}}
	findings := analyzeCrossScope(user, proj, "/u/settings.json", "/p/settings.json")

	var override, restated int
	for _, f := range findings {
		if strings.Contains(f.Issue, "overrides user") {
			override++
		}
		if strings.Contains(f.Issue, "restates the identical value") {
			restated++
		}
	}
	// permissions is a map → skipped by design.
	if override != 1 || restated != 1 || len(findings) != 2 {
		t.Errorf("override=%d restated=%d findings=%+v", override, restated, findings)
	}

	if got := analyzeCrossScope(user, nil, "/u", "/p"); got != nil {
		t.Errorf("missing project settings should yield nil, got %+v", got)
	}
}
