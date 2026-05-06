package cache

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// withTempHome redirects HOME so cache writes happen in a per-test temp dir.
// Also clears TD_NO_CACHE so a developer running tests with the env var
// set globally (to disable cache during interactive use) doesn't see
// false-negative test failures.
func withTempHome(t *testing.T) string {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("TD_NO_CACHE", "")
	return filepath.Join(tmp, ".config", "tokendog", "cache")
}

func TestKeyStability(t *testing.T) {
	withTempHome(t)
	t.Setenv("AWS_PROFILE", "prod")
	k1 := Key("aws", []string{"ec2", "describe-instances"})
	k2 := Key("aws", []string{"ec2", "describe-instances"})
	if k1 != k2 {
		t.Errorf("Key not stable across calls: %s vs %s", k1, k2)
	}
	t.Setenv("AWS_PROFILE", "staging")
	k3 := Key("aws", []string{"ec2", "describe-instances"})
	if k1 == k3 {
		t.Errorf("Key did not change when AWS_PROFILE changed: %s", k1)
	}
}

func TestKeyIgnoresIrrelevantEnv(t *testing.T) {
	withTempHome(t)
	k1 := Key("git", []string{"status"})
	t.Setenv("PATH", "/x:/y")
	t.Setenv("TERM", "xterm-256color")
	k2 := Key("git", []string{"status"})
	if k1 != k2 {
		t.Errorf("Key changed for irrelevant env vars: %s vs %s", k1, k2)
	}
}

func TestSetAndGetWithinTTL(t *testing.T) {
	withTempHome(t)
	key := Key("git", []string{"status"})
	Set(key, Entry{Command: "git status", Output: "clean", RawBytes: 5})
	got, ok := Get(key)
	if !ok {
		t.Fatal("expected cache hit")
	}
	if got.Output != "clean" {
		t.Errorf("Output = %q, want %q", got.Output, "clean")
	}
	if got.OutputSHA == "" {
		t.Error("OutputSHA was not set")
	}
}

func TestExpiredEntryIsMiss(t *testing.T) {
	dir := withTempHome(t)
	t.Setenv(envTTLSeconds, "1")
	key := Key("git", []string{"status"})
	Set(key, Entry{Command: "git status", Output: "clean", RawBytes: 5})

	// Backdate the file to force expiry.
	path := filepath.Join(dir, key+".json")
	old := time.Now().Add(-2 * time.Second)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatalf("chtimes: %v", err)
	}

	if _, ok := Get(key); ok {
		t.Error("expected cache miss for expired entry")
	}
}

func TestDisabledByEnv(t *testing.T) {
	withTempHome(t)
	t.Setenv(envDisable, "1")
	key := Key("git", []string{"status"})
	Set(key, Entry{Command: "git status", Output: "clean"})
	if _, ok := Get(key); ok {
		t.Error("expected miss when TD_NO_CACHE=1")
	}
}

func TestRenderHitFormat(t *testing.T) {
	e := &Entry{RawBytes: 1234, OutputSHA: "abc12345", HitCount: 0}
	out := RenderHit(e, 12*time.Second)
	if !strings.Contains(out, "12s") {
		t.Errorf("expected age in marker, got: %q", out)
	}
	if !strings.Contains(out, "1234 bytes") {
		t.Errorf("expected size in marker, got: %q", out)
	}
	if !strings.Contains(out, "TD_NO_CACHE=1") {
		t.Errorf("expected bypass instruction in marker, got: %q", out)
	}
}

func TestShortDurationBoundaries(t *testing.T) {
	cases := []struct {
		d    time.Duration
		want string
	}{
		{500 * time.Millisecond, "500ms"},
		{15 * time.Second, "15s"},
		{90 * time.Second, "1m30s"},
	}
	for _, tc := range cases {
		if got := shortDuration(tc.d); got != tc.want {
			t.Errorf("shortDuration(%v) = %q, want %q", tc.d, got, tc.want)
		}
	}
}
