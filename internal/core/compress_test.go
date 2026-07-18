package core

import (
	"strings"
	"testing"
)

// elig builds an eligible (last-turn) result.
func elig(cmd, text string) *ToolResult { return &ToolResult{Command: cmd, Text: text, Eligible: true} }

// prior builds a non-eligible (earlier-turn) result.
func prior(cmd, text string) *ToolResult {
	return &ToolResult{Command: cmd, Text: text, Eligible: false}
}

func TestCompressPerToolFilter(t *testing.T) {
	gitStatus := "On branch main\nYour branch is up to date with 'origin/main'.\n\n" +
		"Changes not staged for commit:\n\tmodified:   foo.go\n\tmodified:   bar.go\n"
	r := elig("git status", gitStatus)
	conv := &Conversation{Results: []*ToolResult{r}}

	savings := Compress(conv, Options{Dedup: true})
	if !r.Replaced {
		t.Fatal("expected git status to be filtered")
	}
	if !strings.HasPrefix(r.Replacement, "branch:") {
		t.Errorf("expected compact git status, got %q", r.Replacement)
	}
	if len(savings) != 1 || savings[0].Label != "git status" {
		t.Errorf("unexpected savings: %+v", savings)
	}
}

func TestCompressGenericJSONFallback(t *testing.T) {
	pretty := "{\n  \"a\": 1,\n  \"b\": [1, 2, 3]\n}"
	r := elig("http GET https://x/y", pretty) // no per-tool filter for `http`
	conv := &Conversation{Results: []*ToolResult{r}}

	Compress(conv, Options{Dedup: true})
	if !r.Replaced || strings.Contains(r.Replacement, "\n  ") {
		t.Errorf("expected generic JSON compaction, got replaced=%v %q", r.Replaced, r.Replacement)
	}
}

func TestCompressDedupAcrossTurns(t *testing.T) {
	dup := strings.Repeat("config line with payload\n", 40)
	conv := &Conversation{Results: []*ToolResult{
		prior("cat config.yaml", dup),
		elig("cat config.yaml", dup),
	}}
	savings := Compress(conv, Options{Dedup: true})

	last := conv.Results[1]
	if !last.Replaced || !strings.Contains(last.Replacement, "identical to the output") {
		t.Fatalf("expected dedup back-reference, got %q", last.Replacement)
	}
	if conv.Results[0].Replaced {
		t.Error("earlier (non-eligible) result must never be replaced")
	}
	if len(savings) != 1 || !strings.Contains(savings[0].Label, "dedup") {
		t.Errorf("unexpected dedup savings: %+v", savings)
	}
}

func TestCompressDedupRespectsOptOut(t *testing.T) {
	dup := strings.Repeat("repeated payload line\n", 40)
	conv := &Conversation{Results: []*ToolResult{
		prior("cat x", dup),
		elig("cat x", dup),
	}}
	Compress(conv, Options{Dedup: false}) // dedup disabled
	if conv.Results[1].Replaced && strings.Contains(conv.Results[1].Replacement, "identical to the output") {
		t.Error("dedup fired despite Options.Dedup=false")
	}
}

func TestCompressNonEligibleUntouched(t *testing.T) {
	// Only eligible results may change, even when they'd otherwise filter.
	r := prior("git status", "On branch main\n\tmodified: a.go\n")
	conv := &Conversation{Results: []*ToolResult{r}}
	if savings := Compress(conv, Options{Dedup: true}); len(savings) != 0 {
		t.Errorf("non-eligible result produced savings: %+v", savings)
	}
	if r.Replaced {
		t.Error("non-eligible result was replaced")
	}
}

func TestCompressNonShellOnlyDedup(t *testing.T) {
	// A result with no command (e.g. a file-read tool) is ineligible for
	// per-tool filtering but still dedup-able.
	dup := strings.Repeat("read file content line\n", 40)
	conv := &Conversation{Results: []*ToolResult{
		prior("", dup),
		elig("", dup),
	}}
	Compress(conv, Options{Dedup: true})
	if !conv.Results[1].Replaced {
		t.Error("expected dedup to fire on commandless repeated output")
	}
	// And a unique commandless result is left alone (no filter applies).
	uniq := elig("", "some unique non-json output\n")
	conv2 := &Conversation{Results: []*ToolResult{uniq}}
	Compress(conv2, Options{Dedup: true})
	if uniq.Replaced {
		t.Error("commandless unique output should pass through")
	}
}

func TestCompressEmptyConversation(t *testing.T) {
	if s := Compress(nil, Options{}); s != nil {
		t.Error("nil conversation should yield nil savings")
	}
	if s := Compress(&Conversation{}, Options{}); s != nil {
		t.Error("empty conversation should yield nil savings")
	}
}

// HTML reaching the reversible pass must be handed to the prose sidecar as
// extracted visible text, not raw markup (which looksLikeProse would reject).
func TestApplyReversibleHTMLFeedsCleanTextToProse(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	html := "<!doctype html><html><head><style>.x{color:red}</style>" +
		"<script>track()</script></head><body><h1>Refund status</h1>" +
		"<p>The customer was made whole earlier via a chargeback. A follow-up " +
		"credit was logged but does not match the processor timeline.</p></body></html>"

	var seen string
	prose := func(text string) (string, bool) {
		seen = text // capture exactly what the sidecar received
		return "SUMMARY", true
	}

	out, ok := applyReversible("curl https://x", html, 1, prose)
	if !ok {
		t.Fatal("expected reversible pass to fire")
	}
	if !strings.HasPrefix(out, "SUMMARY") || !strings.Contains(out, "td:STASHED") {
		t.Errorf("expected prose output + retrieval marker, got %q", out)
	}
	if strings.ContainsAny(seen, "<>") || strings.Contains(seen, "track()") || strings.Contains(seen, "color:red") {
		t.Errorf("prose input not cleaned of markup/script/style: %q", seen)
	}
	if !strings.Contains(seen, "Refund status") || !strings.Contains(seen, "chargeback") {
		t.Errorf("prose input lost visible text: %q", seen)
	}
}

// Without a prose sidecar, a short cleaned-HTML preview is returned verbatim
// (too few lines to head/tail elide). Because that text is a lossy reduction,
// it must still carry a retrieval marker or the dropped markup is silently
// unrecoverable.
func TestApplyReversibleHTMLPreviewCarriesMarker(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	// Big enough (noise-heavy) that cleaned text + marker still beats the raw.
	html := "<html><head><style>" + strings.Repeat("a{color:red}", 200) + "</style>" +
		"<script>" + strings.Repeat("doStuff();", 200) + "</script></head>" +
		"<body><p>Short visible sentence.</p></body></html>"

	out, ok := applyReversible("curl https://x", html, 1, nil)
	if !ok {
		t.Fatal("expected reversible pass to fire")
	}
	if !strings.Contains(out, "td:STASHED") {
		t.Errorf("lossy HTML reduction returned without a recovery marker: %q", out)
	}
	if strings.Contains(out, "doStuff") || strings.Contains(out, "color:red") {
		t.Errorf("script/style noise survived: %q", out)
	}
	if !strings.Contains(out, "Short visible sentence.") {
		t.Errorf("visible text lost: %q", out)
	}
	if len(out) >= len(html) {
		t.Errorf("no savings: %d -> %d", len(html), len(out))
	}
}
