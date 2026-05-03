package filter

import (
	"fmt"
	"regexp"
	"strings"
)

const maxWebFetchBytes = 8000

var (
	reScript  = regexp.MustCompile(`(?is)<script[^>]*>.*?</script>`)
	reStyle   = regexp.MustCompile(`(?is)<style[^>]*>.*?</style>`)
	reNav     = regexp.MustCompile(`(?is)<nav[^>]*>.*?</nav>`)
	reHeader  = regexp.MustCompile(`(?is)<header[^>]*>.*?</header>`)
	reFooter  = regexp.MustCompile(`(?is)<footer[^>]*>.*?</footer>`)
	reAside   = regexp.MustCompile(`(?is)<aside[^>]*>.*?</aside>`)
	reTags    = regexp.MustCompile(`<[^>]+>`)
	reSpaces  = regexp.MustCompile(`[ \t]+`)
	reNewlines = regexp.MustCompile(`\n{3,}`)
)

func WebFetch(content string) string {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return content
	}

	switch {
	case strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "["):
		return filterWebJSON(trimmed)
	case strings.Contains(trimmed, "<html") || strings.Contains(trimmed, "<!DOCTYPE") || strings.Contains(trimmed, "</"):
		return filterHTML(trimmed)
	default:
		return truncateText(trimmed)
	}
}

func filterHTML(html string) string {
	// Remove noisy structural blocks
	html = reScript.ReplaceAllString(html, "")
	html = reStyle.ReplaceAllString(html, "")
	html = reNav.ReplaceAllString(html, "")
	html = reHeader.ReplaceAllString(html, "")
	html = reFooter.ReplaceAllString(html, "")
	html = reAside.ReplaceAllString(html, "")

	// Strip remaining tags
	html = reTags.ReplaceAllString(html, " ")

	// Decode common HTML entities
	html = strings.ReplaceAll(html, "&amp;", "&")
	html = strings.ReplaceAll(html, "&lt;", "<")
	html = strings.ReplaceAll(html, "&gt;", ">")
	html = strings.ReplaceAll(html, "&quot;", `"`)
	html = strings.ReplaceAll(html, "&#39;", "'")
	html = strings.ReplaceAll(html, "&nbsp;", " ")

	// Collapse whitespace
	html = reSpaces.ReplaceAllString(html, " ")

	// Clean up lines — drop blank/whitespace-only lines
	lines := strings.Split(html, "\n")
	var kept []string
	for _, line := range lines {
		if t := strings.TrimSpace(line); t != "" {
			kept = append(kept, t)
		}
	}
	html = strings.Join(kept, "\n")
	html = reNewlines.ReplaceAllString(html, "\n\n")

	return truncateText(strings.TrimSpace(html))
}

func filterWebJSON(content string) string {
	// JSON: truncate if oversized, otherwise pass through
	return truncateText(content)
}

func truncateText(s string) string {
	if len(s) <= maxWebFetchBytes {
		return s
	}
	return s[:maxWebFetchBytes] + fmt.Sprintf("\n... [%d bytes truncated]", len(s)-maxWebFetchBytes)
}
