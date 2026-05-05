package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"tokendog/internal/analytics"
	"tokendog/internal/cache"
	"tokendog/internal/filter"
)

// runFiltered is the single funnel for `td <tool> <args>` style commands.
// It owns the cache check, the exec, the filter, the negative-savings
// guard, and the analytics record. Per-tool wrappers should always go
// through this so the four cross-cutting concerns stay in one place.
//
// Behavior:
//  1. Cache check by (binary, args, cwd, sensitive env). On hit within TTL,
//     emit a compact marker and skip exec entirely.
//  2. Exec the wrapped binary, capturing stdout. Stderr streams live so
//     progress and warnings remain visible.
//  3. Apply the per-tool filter, then Guard against any size regression.
//  4. Cache the *filtered* output (smaller, identical to what the model
//     saw) so future hits return what callers expect to see again.
//  5. Record analytics with real token counts via the tokenizer.
func runFiltered(binary string, args []string, fn func(string) string, recordPrefix string) error {
	cmdLabel := recordPrefix + strings.Join(args, " ")
	key := cache.Key(binary, args)

	if hit, ok := cache.Get(key); ok {
		marker := cache.RenderHit(hit, time.Since(hit.Timestamp))
		fmt.Print(marker)
		cache.IncrementHit(key, hit)
		_ = analytics.Save(analytics.NewCacheHitRecord(cmdLabel, hit.RawBytes, marker))
		return nil
	}

	start := time.Now()
	c := exec.Command(binary, args...)
	c.Stderr = os.Stderr
	c.Stdin = os.Stdin
	out, err := c.Output()
	elapsed := time.Since(start).Milliseconds()

	raw := string(out)
	filtered := filter.Guard(raw, fn(raw))
	fmt.Print(filtered)

	// Only cache successful runs. Errors may resolve (transient network,
	// permission flaps) and we don't want stale failures returning silent
	// "cache hits" of broken state.
	if err == nil {
		cache.Set(key, cache.Entry{
			Command:   cmdLabel,
			CWD:       cwdSafe(),
			Timestamp: time.Now(),
			RawBytes:  len(raw),
			Output:    filtered,
		})
	}

	_ = analytics.Save(analytics.NewRecord(cmdLabel, raw, filtered, elapsed))
	return err
}

func cwdSafe() string {
	d, err := os.Getwd()
	if err != nil {
		return ""
	}
	return d
}
