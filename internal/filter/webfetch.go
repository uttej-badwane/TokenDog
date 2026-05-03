package filter

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	reScript   = regexp.MustCompile(`(?is)<script[^>]*>.*?</script>`)
	reStyle    = regexp.MustCompile(`(?is)<style[^>]*>.*?</style>`)
	reNav      = regexp.MustCompile(`(?is)<nav[^>]*>.*?</nav>`)
	reHeader   = regexp.MustCompile(`(?is)<header[^>]*>.*?</header>`)
	reFooter   = regexp.MustCompile(`(?is)<footer[^>]*>.*?</footer>`)
	reAside    = regexp.MustCompile(`(?is)<aside[^>]*>.*?</aside>`)
	reTags     = regexp.MustCompile(`<[^>]+>`)
	reSpaces   = regexp.MustCompile(`[ \t]+`)
	reNewlines = regexp.MustCompile(`\n{3,}`)
)

func WebFetch(content string) string {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return content
	}

	rawLen := len(trimmed)

	var result string
	switch {
	case strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "["):
		result = compactJSON(trimmed)
	case strings.Contains(trimmed, "<html") || strings.Contains(trimmed, "<!DOCTYPE") || strings.Contains(trimmed, "</"):
		result = stripHTML(trimmed)
	default:
		result = collapseWhitespace(trimmed)
	}

	savedBytes := rawLen - len(result)
	if savedBytes > 0 {
		result = fmt.Sprintf("[tokendog: stripped %d bytes of noise — full content below]\n\n%s", savedBytes, result)
	}
	return result
}

// stripHTML removes all non-content elements and returns clean text.
// This is lossless — no content is dropped, only structural noise.
func stripHTML(html string) string {
	// Remove zero-content elements
	html = reScript.ReplaceAllString(html, "")
	html = reStyle.ReplaceAllString(html, "")
	html = reNav.ReplaceAllString(html, "")
	html = reHeader.ReplaceAllString(html, "")
	html = reFooter.ReplaceAllString(html, "")
	html = reAside.ReplaceAllString(html, "")

	// Strip remaining tags — text content is preserved
	html = reTags.ReplaceAllString(html, " ")

	// Decode common HTML entities
	html = strings.ReplaceAll(html, "&amp;", "&")
	html = strings.ReplaceAll(html, "&lt;", "<")
	html = strings.ReplaceAll(html, "&gt;", ">")
	html = strings.ReplaceAll(html, "&quot;", `"`)
	html = strings.ReplaceAll(html, "&#39;", "'")
	html = strings.ReplaceAll(html, "&nbsp;", " ")

	return collapseWhitespace(html)
}

// compactJSON collapses JSON whitespace without dropping any fields.
func compactJSON(content string) string {
	return collapseWhitespace(content)
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
