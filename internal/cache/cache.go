// Package cache implements a short-TTL command output cache. When Claude (or
// any caller) repeats the exact same command within the TTL window, td
// returns a compact "identical to prior call" marker instead of the full
// output. This is the highest-leverage savings for workflows dominated by
// small repeated calls (`git rev-parse HEAD`, `gh pr view <n>`, AWS lookups)
// where structural filters have nothing to compress.
//
// Safety contract:
//   - TTL is short (30s default) so genuine state-change polling within a
//     single Claude turn isn't masked.
//   - Errors (non-zero exit) are never cached — they may resolve.
//   - The hit marker tells the model when the prior call was made and how to
//     bypass cache (TD_NO_CACHE=1), so it can choose to refresh.
//   - Cache key includes cwd + sensitive env vars (AWS_PROFILE, KUBECONFIG,
//     GITHUB_TOKEN, etc.) — switching context never returns stale output.
package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	defaultTTL    = 30 * time.Second
	envDisable    = "TD_NO_CACHE"
	envTTLSeconds = "TD_CACHE_TTL"
	pruneAge      = 1 * time.Hour
)

// envVarsThatAffectOutput is the allowlist of environment variables included
// in the cache key. Anything outside this list (PATH, PWD, terminal env,
// etc.) is excluded so unrelated noise doesn't blow the cache.
var envVarsThatAffectOutput = []string{
	// AWS
	"AWS_PROFILE", "AWS_REGION", "AWS_DEFAULT_REGION", "AWS_ACCESS_KEY_ID",
	"AWS_SESSION_TOKEN", "AWS_ROLE_ARN",
	// GCP
	"GOOGLE_APPLICATION_CREDENTIALS", "GCP_PROJECT", "CLOUDSDK_CORE_PROJECT",
	"CLOUDSDK_CORE_ACCOUNT",
	// Azure
	"AZURE_SUBSCRIPTION_ID", "AZURE_TENANT_ID",
	// Kubernetes
	"KUBECONFIG", "KUBE_CONTEXT", "KUBE_NAMESPACE",
	// GitHub
	"GH_TOKEN", "GITHUB_TOKEN", "GH_HOST", "GH_REPO",
	// Git
	"GIT_DIR", "GIT_WORK_TREE",
}

// Entry is the on-disk cache record.
type Entry struct {
	Command   string    `json:"command"`
	CWD       string    `json:"cwd"`
	Timestamp time.Time `json:"timestamp"`
	RawBytes  int       `json:"raw_bytes"`
	Output    string    `json:"output"`
	OutputSHA string    `json:"output_sha"`
	HitCount  int       `json:"hit_count"`
}

// Disabled reports whether caching is turned off for this process.
func Disabled() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(envDisable)))
	return v == "1" || v == "true" || v == "yes"
}

// TTL returns the configured cache TTL, falling back to the 30s default.
func TTL() time.Duration {
	if s := os.Getenv(envTTLSeconds); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			return time.Duration(n) * time.Second
		}
	}
	return defaultTTL
}

// Key derives a stable cache key from binary, args, cwd, and the env-var
// allowlist. Empty values are included so e.g. unset AWS_PROFILE is distinct
// from AWS_PROFILE=default.
func Key(binary string, args []string) string {
	cwd, _ := os.Getwd()

	envPairs := make([]string, 0, len(envVarsThatAffectOutput))
	for _, name := range envVarsThatAffectOutput {
		envPairs = append(envPairs, name+"="+os.Getenv(name))
	}
	sort.Strings(envPairs)

	h := sha256.New()
	h.Write([]byte(binary))
	h.Write([]byte{0})
	for _, a := range args {
		h.Write([]byte(a))
		h.Write([]byte{0})
	}
	h.Write([]byte("|cwd:"))
	h.Write([]byte(cwd))
	h.Write([]byte{0})
	for _, p := range envPairs {
		h.Write([]byte(p))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

func dir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	d := filepath.Join(home, ".config", "tokendog", "cache")
	if err := os.MkdirAll(d, 0755); err != nil {
		return "", err
	}
	return d, nil
}

// Get returns a cached entry for key if one exists and is younger than TTL.
// On any error (missing file, malformed JSON, expired) returns (nil, false).
func Get(key string) (*Entry, bool) {
	if Disabled() {
		return nil, false
	}
	d, err := dir()
	if err != nil {
		return nil, false
	}
	path := filepath.Join(d, key+".json")
	info, err := os.Stat(path)
	if err != nil {
		return nil, false
	}
	if time.Since(info.ModTime()) > TTL() {
		return nil, false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	var e Entry
	if err := json.Unmarshal(data, &e); err != nil {
		return nil, false
	}
	return &e, true
}

// Set writes an entry. The hit count is preserved if the file already exists
// so repeated cache HITS within a TTL window keep counting up.
func Set(key string, e Entry) {
	if Disabled() {
		return
	}
	d, err := dir()
	if err != nil {
		return
	}
	if e.OutputSHA == "" {
		sum := sha256.Sum256([]byte(e.Output))
		e.OutputSHA = hex.EncodeToString(sum[:8])
	}
	data, err := json.Marshal(e)
	if err != nil {
		return
	}
	_ = os.WriteFile(filepath.Join(d, key+".json"), data, 0644)
	// Best-effort prune of expired entries; ignore errors.
	go prune(d)
}

// IncrementHit re-saves an entry with HitCount+1 and bumps mtime so the TTL
// window slides on each repeated call. Without this, a tight loop of repeats
// could exhaust the original 30s window.
func IncrementHit(key string, e *Entry) {
	if e == nil {
		return
	}
	e.HitCount++
	e.Timestamp = time.Now()
	Set(key, *e)
}

// RenderHit returns the compact marker shown to the model on cache hit. The
// marker is intentionally short — that's the whole point — but contains
// enough metadata that the model can reason about staleness and refresh if
// needed.
func RenderHit(e *Entry, age time.Duration) string {
	return fmt.Sprintf("[td cache: identical output %s ago, %d bytes, sha256=%s, hit #%d. TD_NO_CACHE=1 to bypass]\n",
		shortDuration(age), e.RawBytes, e.OutputSHA, e.HitCount+1)
}

func shortDuration(d time.Duration) string {
	switch {
	case d < time.Second:
		return fmt.Sprintf("%dms", d.Milliseconds())
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	default:
		return fmt.Sprintf("%dm%ds", int(d.Minutes()), int(d.Seconds())%60)
	}
}

// prune removes cache files older than pruneAge. Best-effort and bounded —
// ignores errors and walks only the cache dir.
func prune(d string) {
	entries, err := os.ReadDir(d)
	if err != nil {
		return
	}
	now := time.Now()
	for _, ent := range entries {
		if ent.IsDir() {
			continue
		}
		info, err := ent.Info()
		if err != nil {
			continue
		}
		if now.Sub(info.ModTime()) > pruneAge {
			_ = os.Remove(filepath.Join(d, ent.Name()))
		}
	}
}
