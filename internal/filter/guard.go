package filter

// Guard returns filtered if it's strictly smaller than raw, else raw. This is
// the universal contract: a filter must never produce more bytes than its
// input. Individual filters already try to enforce this internally, but the
// guard is the cheap last line of defense — if a filter regresses, td gain
// won't show negative-savings rows for users.
func Guard(raw, filtered string) string {
	if len(filtered) >= len(raw) {
		return raw
	}
	return filtered
}
