package filter

// WebSearch compresses WebSearch tool output by collapsing redundant
// whitespace. Lossless — all titles, URLs, and snippets preserved.
func WebSearch(content string) string {
	return collapseWhitespace(content)
}
