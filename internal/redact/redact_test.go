package redact

import (
	"strings"
	"testing"
)

func TestRedactAWSAccessKey(t *testing.T) {
	in := "key=AKIAIOSFODNN7EXAMPLE other text"
	out, n := All(in)
	if n != 1 {
		t.Errorf("expected 1 redaction, got %d", n)
	}
	if strings.Contains(out, "AKIA") {
		t.Errorf("AWS key not redacted: %q", out)
	}
	if !strings.Contains(out, "[REDACTED-AWS-KEY]") {
		t.Errorf("placeholder missing: %q", out)
	}
}

func TestRedactGithubToken(t *testing.T) {
	cases := []string{
		"token=ghp_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"token=ghs_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		"token=gho_cccccccccccccccccccccccccccccccccccc",
	}
	for _, in := range cases {
		out, n := All(in)
		if n == 0 {
			t.Errorf("github token not detected: %q", in)
		}
		if !strings.Contains(out, "[REDACTED-GITHUB-TOKEN]") {
			t.Errorf("placeholder missing: %q -> %q", in, out)
		}
	}
}

func TestRedactJWT(t *testing.T) {
	in := "Bearer eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiJ0ZXN0In0.signature"
	out, _ := All(in)
	if strings.Contains(out, "eyJ") {
		t.Errorf("JWT not redacted: %q", out)
	}
}

func TestRedactPEMBlock(t *testing.T) {
	in := `prefix
-----BEGIN RSA PRIVATE KEY-----
MIIEowIBAAKCAQEA1234567890abcdef
abcdef1234567890abcdef1234567890
-----END RSA PRIVATE KEY-----
suffix`
	out, n := All(in)
	if n != 1 {
		t.Errorf("expected 1 PEM block redaction, got %d", n)
	}
	if strings.Contains(out, "MIIEow") {
		t.Errorf("PEM body leaked: %q", out)
	}
	if !strings.Contains(out, "prefix") || !strings.Contains(out, "suffix") {
		t.Errorf("non-secret content was clobbered: %q", out)
	}
}

func TestRedactBearer(t *testing.T) {
	in := "Authorization: Bearer abc123def456ghi789jkl"
	out, _ := All(in)
	if strings.Contains(out, "abc123def456ghi789jkl") {
		t.Errorf("bearer token not redacted: %q", out)
	}
	if !strings.Contains(out, "[REDACTED]") {
		t.Errorf("bearer placeholder missing: %q", out)
	}
}

func TestNoFalsePositiveOnNormalText(t *testing.T) {
	cases := []string{
		"this is just some normal text",
		"// AKIA is a prefix used by AWS but not a key",
		"package github.com/foo/bar",
	}
	for _, in := range cases {
		out, n := All(in)
		if n != 0 {
			t.Errorf("false positive on %q: %d redactions, output %q", in, n, out)
		}
		if out != in {
			t.Errorf("text modified despite no match: %q -> %q", in, out)
		}
	}
}

func TestHasSecret(t *testing.T) {
	if HasSecret("nothing here") {
		t.Error("HasSecret returned true for clean text")
	}
	if !HasSecret("AKIAIOSFODNN7EXAMPLE") {
		t.Error("HasSecret missed AWS key")
	}
}

func TestRedactAnthropicKey(t *testing.T) {
	in := "env ANTHROPIC_API_KEY=sk-ant-api03-aaaaaaaaaaaaaaaaaaaaaaaa done"
	out, n := All(in)
	if n != 1 {
		t.Errorf("expected 1 redaction, got %d", n)
	}
	if strings.Contains(out, "sk-ant-") {
		t.Errorf("anthropic key not redacted: %q", out)
	}
	if !strings.Contains(out, "[REDACTED-ANTHROPIC-KEY]") {
		t.Errorf("placeholder missing: %q", out)
	}
	// Short sk-ant- prefixes in prose must NOT fire.
	if _, n := All("the sk-ant- prefix marks Anthropic keys"); n != 0 {
		t.Errorf("bare prefix should not redact")
	}
}

func TestFindNames(t *testing.T) {
	names := FindNames("key sk-ant-api03-aaaaaaaaaaaaaaaaaaaaaaaa and AKIAIOSFODNN7EXAMPLE")
	if len(names) != 2 {
		t.Fatalf("expected 2 pattern names, got %v", names)
	}
	if names[0] != "anthropic-api-key" || names[1] != "aws-access-key" {
		t.Errorf("unexpected names: %v", names)
	}
	if got := FindNames("perfectly ordinary text"); got != nil {
		t.Errorf("clean text should yield nil, got %v", got)
	}
}
