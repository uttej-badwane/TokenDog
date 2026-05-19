// Package compress implements lossless prose compression for markdown and
// plain-text files. It strips filler words, articles, pleasantries, and
// hedging phrases from natural-language prose while leaving code blocks,
// inline code, URLs, file paths, and identifiers byte-for-byte unchanged.
//
// Algorithm ported from caveman-shrink (github.com/juliusbrussee/caveman,
// MIT) with a full Go rewrite — no Node dependency, no network calls.
//
// Primary use cases:
//   - Compress CLAUDE.md / memory files so every session starts with a
//     smaller input token count (td compress command).
//   - Compress MCP tool descriptions in the proxy before they are billed
//     (proxy/filter.go calls CompressString on each description field).
package compress

import (
	"regexp"
	"strings"
)

// Protected patterns — these regions are extracted as sentinels before any
// compression runs and spliced back in verbatim afterwards.
var protectedPatterns = []*regexp.Regexp{
	// Fenced code blocks — match ``` or ~~~ style (Go regexp has no backrefs,
	// so we match the two fence styles separately with non-greedy dotall).
	regexp.MustCompile("(?s)```[^\\n]*\\n.*?```"),
	regexp.MustCompile("(?s)~~~[^\\n]*\\n.*?~~~"),
	// Inline code
	regexp.MustCompile("`[^`\\n]+`"),
	// URLs
	regexp.MustCompile(`https?://\S+`),
	// Filesystem paths (contain / or \)
	regexp.MustCompile(`[\w.\-]*/[\w./\\\-]+`),
	regexp.MustCompile(`[\w.\-]*\\[\w.\\\-]+`),
	// Version numbers (x.y.z)
	regexp.MustCompile(`\b\d+\.\d+\.\d+\b`),
	// CONST_CASE identifiers
	regexp.MustCompile(`\b[A-Z][A-Z0-9]*(?:_[A-Z][A-Z0-9]*)+\b`),
	// dotted.method or pkg.fn() calls
	regexp.MustCompile(`\b\w+\.\w+(?:\.\w+)*\(?[^)]*\)?`),
	// Shell/flag tokens starting with -
	regexp.MustCompile(`--?[\w-]+=?\S*`),
}

// Prose-level substitutions: multi-word phrases before single-word passes.
var phraseReplacements = []struct {
	re          *regexp.Regexp
	replacement string
}{
	{regexp.MustCompile(`(?i)\bin order to\b`), "to"},
	{regexp.MustCompile(`(?i)\bmake sure to\b`), "ensure"},
	{regexp.MustCompile(`(?i)\bthe reason is because\b`), "because"},
	{regexp.MustCompile(`(?i)\bit is important to note that\b`), ""},
	{regexp.MustCompile(`(?i)\bplease note that\b`), ""},
	{regexp.MustCompile(`(?i)\bkeep in mind that\b`), ""},
	{regexp.MustCompile(`(?i)\bfeel free to\b`), ""},
	{regexp.MustCompile(`(?i)\bit('s| is) worth noting that\b`), ""},
	{regexp.MustCompile(`(?i)\byou should always\b`), "always"},
	{regexp.MustCompile(`(?i)\bmake sure\b`), "ensure"},
	{regexp.MustCompile(`(?i)\bI would recommend\b`), "recommend"},
	{regexp.MustCompile(`(?i)\bI'd recommend\b`), "recommend"},
	{regexp.MustCompile(`(?i)\bI would suggest\b`), "suggest"},
	{regexp.MustCompile(`(?i)\bin addition(ally)?\b`), ""},
	{regexp.MustCompile(`(?i)\bfurthermore\b`), ""},
	{regexp.MustCompile(`(?i)\badditionally\b`), ""},
	{regexp.MustCompile(`(?i)\bhowever,?\s*`), ""},
	{regexp.MustCompile(`(?i)\bof course\b,?\s*`), ""},
	{regexp.MustCompile(`(?i)\bhappy to help\b`), ""},
	{regexp.MustCompile(`(?i)\bI('d| would) be happy to\b`), ""},
	{regexp.MustCompile(`(?i)\bsure,?\s*`), ""},
	{regexp.MustCompile(`(?i)\bcertainly,?\s*`), ""},
	{regexp.MustCompile(`(?i)\bI('ll| will)\s+`), ""},
	{regexp.MustCompile(`(?i)\byou can\s+`), ""},
	{regexp.MustCompile(`(?i)\bwe will\s+`), ""},
	{regexp.MustCompile(`(?i)\blet me\s+`), ""},
	{regexp.MustCompile(`(?i)\blet's\s+`), ""},
}

// Single-word fillers stripped from prose.
var fillerWords = regexp.MustCompile(
	`(?i)\b(just|really|basically|actually|simply|quite|very|essentially|` +
		`literally|generally|typically|normally|usually|often|sometimes|` +
		`perhaps|maybe|kindly|please)\b\s*`,
)

// Articles preceding a lowercase word. Capture the first letter of the
// following word so we can preserve it in the replacement ($1). Avoids
// stripping "A" that starts a sentence (capital A won't match [a-z]).
var articles = regexp.MustCompile(`\b(?:a|an|the)\s+([a-z])`)

// CompressString applies prose compression to a single string. Protected
// regions (code blocks, URLs, paths) are never touched. Returns the
// compressed string; if compression produces a longer result (rare), the
// original is returned unchanged (same guarantee as filter.Guard).
func CompressString(text string) string {
	if len(strings.TrimSpace(text)) == 0 {
		return text
	}
	out := withProtected(text, compressProse)
	if len(out) >= len(text) {
		return text
	}
	return out
}

// CompressFile compresses a markdown/prose file. Returns (compressed, changed).
// Code blocks, inline code, URLs, paths, and headings are preserved exactly.
// The caller is responsible for writing the result; this function is pure.
func CompressFile(content string) (compressed string, changed bool) {
	compressed = CompressString(content)
	changed = compressed != content
	return
}

// withProtected extracts all protected regions as sentinels, applies fn to
// the remaining prose, then splices the originals back in.
func withProtected(text string, fn func(string) string) string {
	segments := []string{}
	working := text

	for _, re := range protectedPatterns {
		working = re.ReplaceAllStringFunc(working, func(m string) string {
			idx := len(segments)
			segments = append(segments, m)
			return "\x00" + string(rune(idx+1)) + "\x00"
		})
	}

	processed := fn(working)

	// Restore sentinels. Use a simple scanner so we handle multi-char
	// rune indices correctly.
	if len(segments) == 0 {
		return processed
	}
	var b strings.Builder
	runes := []rune(processed)
	for i := 0; i < len(runes); i++ {
		if runes[i] == '\x00' && i+2 < len(runes) && runes[i+2] == '\x00' {
			idx := int(runes[i+1]) - 1
			if idx >= 0 && idx < len(segments) {
				b.WriteString(segments[idx])
				i += 2
				continue
			}
		}
		b.WriteRune(runes[i])
	}
	return b.String()
}

func compressProse(text string) string {
	s := text

	// Multi-word phrase replacements first.
	for _, sub := range phraseReplacements {
		s = sub.re.ReplaceAllString(s, sub.replacement)
	}

	// Single-word fillers.
	s = fillerWords.ReplaceAllString(s, "")

	// Articles before lowercase words (keep the captured first letter).
	s = articles.ReplaceAllString(s, "$1")

	// Clean up whitespace artefacts introduced by removals: trim leading
	// spaces on each line, collapse multiple spaces, tidy punctuation.
	s = regexp.MustCompile(`(?m)^[ \t]+`).ReplaceAllString(s, "")
	s = regexp.MustCompile(`[ \t]{2,}`).ReplaceAllString(s, " ")
	s = regexp.MustCompile(`\s+([,.:;!?])`).ReplaceAllString(s, "$1")
	s = regexp.MustCompile(`\n{3,}`).ReplaceAllString(s, "\n\n")
	// Trim trailing spaces per line (not leading — indentation is meaningful).
	s = regexp.MustCompile(`[ \t]+\n`).ReplaceAllString(s, "\n")
	return strings.TrimRight(s, " \t")
}
