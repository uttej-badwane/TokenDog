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
