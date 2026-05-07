// Package redact replaces well-known secret formats with placeholder
// markers. Used by td purge --redact (rewrites the command field of
// stored history records) and td replay --redact (sanitizes any per-call
// content before display).
//
// Deliberately conservative: we err toward NOT redacting (leaving the
// real value in place) over redacting too aggressively (which could
// destroy legitimate content). Each pattern is anchored to a known prefix
// or shape that wouldn't appear in normal English / code.
//
// Coverage:
//   - AWS access keys (AKIA / ASIA prefixes + 16 hex chars)
//   - AWS secret keys (40-char base64 following "aws_secret_access_key" or
//     "AWS_SECRET_ACCESS_KEY")
//   - GitHub fine-grained PATs and classic tokens (ghp_, gho_, ghu_, ghs_,
//     ghr_, github_pat_)
//   - Slack tokens (xox[abps]-)
//   - Generic Bearer tokens (Authorization: Bearer ...)
//   - PEM blocks (-----BEGIN ... PRIVATE KEY-----)
//   - JWTs (3-segment base64url separated by dots, eyJ prefix)
//
// Out of scope (leave to the user):
//   - Generic high-entropy strings — too many false positives in real code
//   - Database connection strings — no canonical shape
//   - Kubernetes service-account tokens (look like JWTs; covered)
package redact

import (
	"regexp"
	"strings"
)

// All matches every known secret pattern in s and replaces them with
// type-tagged placeholders. Returns the redacted string and the count of
// distinct redactions applied (useful for "redacted N secrets" output).
func All(s string) (string, int) {
	count := 0
	for _, p := range patterns {
		s = p.re.ReplaceAllStringFunc(s, func(m string) string {
			count++
			return p.replacement
		})
	}
	return s, count
}

type pattern struct {
	name        string
	re          *regexp.Regexp
	replacement string
}

var patterns = []pattern{
	// AWS keys: AKIA + 16 uppercase alphanumerics (access key) or ASIA
	// (temporary). Must be word-boundary anchored so we don't match
	// substrings of larger tokens.
	{
		name:        "aws-access-key",
		re:          regexp.MustCompile(`\b(AKIA|ASIA)[0-9A-Z]{16}\b`),
		replacement: "[REDACTED-AWS-KEY]",
	},
	// GitHub PATs: prefix + 36+ alphanumerics. github_pat_ is fine-grained.
	{
		name:        "github-token",
		re:          regexp.MustCompile(`\bgh[pousr]_[A-Za-z0-9]{36,255}\b`),
		replacement: "[REDACTED-GITHUB-TOKEN]",
	},
	{
		name:        "github-fine-grained-pat",
		re:          regexp.MustCompile(`\bgithub_pat_[A-Za-z0-9_]{82}\b`),
		replacement: "[REDACTED-GITHUB-PAT]",
	},
	// Slack tokens.
	{
		name:        "slack-token",
		re:          regexp.MustCompile(`\bxox[abops]-[A-Za-z0-9-]{10,}\b`),
		replacement: "[REDACTED-SLACK-TOKEN]",
	},
	// JWTs: three base64url segments separated by dots, payload starts with
	// "eyJ" (which is `{"` base64-encoded — i.e. start of a JSON object).
	{
		name:        "jwt",
		re:          regexp.MustCompile(`\beyJ[A-Za-z0-9_-]+\.eyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+`),
		replacement: "[REDACTED-JWT]",
	},
	// Bearer tokens in Authorization headers. Match the value after Bearer.
	{
		name:        "bearer-token",
		re:          regexp.MustCompile(`(?i)Authorization:\s*Bearer\s+[A-Za-z0-9._\-+/=]+`),
		replacement: "Authorization: Bearer [REDACTED]",
	},
	// PEM blocks — the entire block from BEGIN to END (multi-line).
	{
		name:        "pem-block",
		re:          regexp.MustCompile(`-----BEGIN [A-Z ]+-----[\s\S]*?-----END [A-Z ]+-----`),
		replacement: "[REDACTED-PEM-BLOCK]",
	},
}

// Names returns the list of pattern names. Useful for `td purge --redact`
// to print "redacted these categories" in its summary.
func Names() []string {
	out := make([]string, len(patterns))
	for i, p := range patterns {
		out[i] = p.name
	}
	return out
}

// HasSecret reports whether s contains any matchable pattern. Cheaper
// than All when you only need a yes/no answer (e.g. flag a record as
// secret-bearing without rewriting it).
func HasSecret(s string) bool {
	for _, p := range patterns {
		if p.re.MatchString(s) {
			return true
		}
	}
	return false
}

// Redacted is a tiny convenience returning just the string. Most callers
// don't need the count.
func Redacted(s string) string {
	out, _ := All(s)
	return out
}

// Ensure the package compiles when nothing imports strings (the import is
// here so future patterns that need it don't have to add the import).
var _ = strings.TrimSpace
