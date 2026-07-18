package filter

import (
	"html"
	"regexp"
	"strings"
)

var (
	reSpaces   = regexp.MustCompile(`[ \t]+`)
	reNewlines = regexp.MustCompile(`\n{3,}`)

	// Blocks whose *content* is never visible text — script/style/noscript
	// bodies and HTML comments. Removing them is lossless with respect to
	// meaningful content (the same category as git.go dropping "index
	// abc..def" headers), so it is safe even on the recovery-less CLI filter
	// path. RE2 has no backreferences, so each tag is matched explicitly.
	reScript  = regexp.MustCompile(`(?is)<script\b[^>]*>.*?</script\s*>`)
	reStyle   = regexp.MustCompile(`(?is)<style\b[^>]*>.*?</style\s*>`)
	reNoScr   = regexp.MustCompile(`(?is)<noscript\b[^>]*>.*?</noscript\s*>`)
	reComment = regexp.MustCompile(`(?s)<!--.*?-->`)

	// Block-level tags become newlines so paragraph/heading/list structure
	// survives as line breaks; every remaining tag becomes a single space so
	// adjacent words never fuse (<b>Hello</b>world -> "Hello world"). Used only
	// by HTMLToText, which is lossy and runs in the reversible pass alone.
	reBlockTag = regexp.MustCompile(`(?i)</?(p|div|br|hr|li|ul|ol|dl|dd|dt|tr|table|thead|tbody|section|article|aside|header|footer|nav|main|figure|figcaption|h[1-6]|blockquote|pre)\b[^>]*>`)
	reAnyTag   = regexp.MustCompile(`(?s)<[^>]+>`)
)

// Curl compresses curl response output. JSON bodies are compacted losslessly
// (every key and value preserved). HTML bodies have script/style/comment noise
// stripped — also lossless, since none of it is visible content — before the
// whitespace collapse. Everything else is just whitespace-collapsed.
//
// The aggressive HTML->text reduction (dropping markup down to visible text)
// is deliberately NOT done here: this path has no stash, so a lossy transform
// would be unrecoverable and violate the lossless contract. That reduction
// lives in HTMLToText, invoked only from the reversible pass where the full
// original is stashed and pullable via td_retrieve.
func Curl(content string) string {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return content
	}

	// Heuristic: looks like JSON if the first char is { or [.
	if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
		// Same lossless compactor as jq — values preserved verbatim.
		return compactJSONValue(trimmed)
	}

	if LooksLikeHTML(trimmed) {
		trimmed = stripHTMLNoise(trimmed)
	}

	return collapseWhitespace(trimmed)
}

// stripHTMLNoise removes script/style/noscript blocks and HTML comments. All
// of that is invisible to a reader, so removing it loses no meaningful content
// — the visible text and the surrounding markup are left untouched.
func stripHTMLNoise(s string) string {
	s = reScript.ReplaceAllString(s, "")
	s = reStyle.ReplaceAllString(s, "")
	s = reNoScr.ReplaceAllString(s, "")
	s = reComment.ReplaceAllString(s, "")
	return s
}

// LooksLikeHTML reports whether a response body is HTML worth reducing. It is
// conservative: the body must start with '<' and carry a real HTML signal, so
// XML documents and stray angle brackets fall through to the lossless path
// rather than being mangled by tag stripping.
func LooksLikeHTML(s string) bool {
	t := strings.TrimLeft(s, " \t\r\n\ufeff")
	if len(t) == 0 || t[0] != '<' {
		return false
	}
	head := t
	if len(head) > 1024 {
		head = head[:1024]
	}
	head = strings.ToLower(head)
	return strings.HasPrefix(head, "<!doctype html") ||
		strings.HasPrefix(head, "<html") ||
		strings.Contains(head, "<head") ||
		strings.Contains(head, "<body") ||
		strings.Contains(head, "<div") ||
		strings.Contains(head, "<meta") ||
		strings.Contains(head, "<p>")
}

// HTMLToText reduces an HTML document to its visible text: script/style/comment
// noise is dropped, block tags become line breaks, remaining tags become
// spaces, and entities are decoded. It is LOSSY (markup, attributes, and link
// targets are discarded), so callers MUST only use it where the original is
// recoverable — the reversible stash pass, never the plain CLI filter path.
// Dep-free (regexp + stdlib html), honoring the package's no-external-deps rule.
func HTMLToText(s string) string {
	s = stripHTMLNoise(s)
	s = reBlockTag.ReplaceAllString(s, "\n")
	s = reAnyTag.ReplaceAllString(s, " ")
	s = html.UnescapeString(s)
	return collapseWhitespace(s)
}

// collapseWhitespace removes redundant whitespace — lossless.
func collapseWhitespace(s string) string {
	s = reSpaces.ReplaceAllString(s, " ")
	lines := strings.Split(s, "\n")
	var kept []string
	for _, line := range lines {
		if t := strings.TrimSpace(line); t != "" {
			kept = append(kept, t)
		}
	}
	s = strings.Join(kept, "\n")
	s = reNewlines.ReplaceAllString(s, "\n\n")
	return strings.TrimSpace(s)
}
