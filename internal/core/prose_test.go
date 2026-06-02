package core

import (
	"strings"
	"testing"

	"tokendog/internal/stash"
)

func TestLooksLikeProse(t *testing.T) {
	prose := "During the incident the gateway began returning errors and we traced " +
		"the cause to a connection leak in the new middleware that never released " +
		"its database handles back to the shared pool."
	if !looksLikeProse(prose) {
		t.Error("a natural-language paragraph should be detected as prose")
	}
	for name, s := range map[string]string{
		"json":  `{"id":"i-0abc123","state":"running","ip":"10.0.1.42"}`,
		"xml":   "<config><host>0.0.0.0</host><port>8080</port></config>",
		"log":   "2026-06-01T10:00:00 INFO svc boot pid=4821\n2026-06-01T10:00:01 WARN svc heap=94%\n2026-06-01T10:00:02 ERROR svc oom\n",
		"empty": "   ",
	} {
		if looksLikeProse(s) {
			t.Errorf("%s should NOT be detected as prose", name)
		}
	}
}

func bigProse() string {
	// Multi-line, > stash MinSize (2048), letter-heavy, long lines →
	// looksLikeProse true, and enough lines that the head/tail fallback can
	// actually elide some.
	line := "The investigation took most of the afternoon because the symptom and the " +
		"root cause turned out to be several layers apart in the system.\n"
	return strings.Repeat(line, 40)
}

func TestProseRouteUsedInReversible(t *testing.T) {
	t.Setenv("HOME", t.TempDir()) // isolate the stash
	content := bigProse()
	called := false
	fake := func(text string) (string, bool) {
		called = true
		if text != content {
			t.Error("prose func received different text than the original")
		}
		return "COMPRESSED PROSE SUMMARY of the incident.", true
	}

	r := &ToolResult{Command: "render-doc", Text: content, Eligible: true}
	conv := &Conversation{Results: []*ToolResult{r}}
	Compress(conv, Options{Reversible: true, Prose: fake})

	if !called {
		t.Fatal("prose compressor was not invoked on prose content")
	}
	if !r.Replaced {
		t.Fatal("result was not replaced")
	}
	if !strings.HasPrefix(r.Replacement, "COMPRESSED PROSE SUMMARY") {
		t.Errorf("replacement should be the prose compression, got: %q", r.Replacement[:40])
	}
	if !strings.Contains(r.Replacement, "[td:STASHED id=") {
		t.Error("prose replacement must carry the recoverability marker")
	}
	// The full original must be recoverable from the stash.
	if rec, ok := stash.Get(stash.ID(content)); !ok || rec.Content != content {
		t.Error("original prose not recoverable from the stash")
	}
}

func TestProseRouteSkippedOnNonProse(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	// A large LOG (not prose): prose func must NOT be called; falls back to
	// the head/tail preview.
	var b strings.Builder
	for i := 0; i < 60; i++ {
		b.WriteString("2026-06-01T10:00:00 INFO svc request handled in 12ms status=200 bytes=4096\n")
	}
	called := false
	fake := func(string) (string, bool) { called = true; return "x", true }

	r := &ToolResult{Command: "journalctl", Text: b.String(), Eligible: true}
	Compress(&Conversation{Results: []*ToolResult{r}}, Options{Reversible: true, Prose: fake})

	if called {
		t.Error("prose compressor must not run on log/structured content")
	}
	if r.Replaced && !strings.Contains(r.Replacement, "[td:STASHED") {
		t.Error("non-prose should still use the head/tail reversible preview")
	}
}

func TestProseFallbackWhenCompressorDeclines(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	content := bigProse()
	decline := func(string) (string, bool) { return "", false } // sidecar down / no help

	r := &ToolResult{Command: "render-doc", Text: content, Eligible: true}
	Compress(&Conversation{Results: []*ToolResult{r}}, Options{Reversible: true, Prose: decline})

	if !r.Replaced || strings.HasPrefix(r.Replacement, "COMPRESSED") {
		t.Error("a declining prose compressor should fall back to the head/tail preview")
	}
	if !strings.Contains(r.Replacement, "[td:STASHED") {
		t.Error("fallback preview must still be recoverable")
	}
}
