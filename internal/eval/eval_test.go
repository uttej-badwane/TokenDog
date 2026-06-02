package eval

import (
	"strings"
	"testing"
)

func mk(name, cmd, output string, keep ...string) Fixture {
	return Fixture{Name: name, Command: cmd, Output: output, MustKeep: keep}
}

func TestRunLosslessPreservesFacts(t *testing.T) {
	f := mk("git", "git status",
		"On branch main\nChanges not staged for commit:\n"+
			"\tmodified:   foo.go\n\tmodified:   bar.go\n",
		"main", "foo.go", "bar.go")
	rep := Run([]Fixture{f})
	if !rep.Pass {
		t.Fatal("lossless git status should preserve every fact")
	}
	r := rep.Results[0]
	if r.Inline != 3 || r.Recoverable != 3 {
		t.Errorf("lossless should be inline==recoverable==3, got inline=%d recover=%d", r.Inline, r.Recoverable)
	}
	if r.Transform != "lossless" {
		t.Errorf("transform = %q, want lossless", r.Transform)
	}
	if r.CompBytes >= r.RawBytes {
		t.Errorf("expected compression, %d -> %d", r.RawBytes, r.CompBytes)
	}
}

func TestRunReversibleRecoverableNotInline(t *testing.T) {
	t.Setenv("HOME", t.TempDir()) // isolate the stash
	var b strings.Builder
	for i := 0; i < 40; i++ {
		b.WriteString("filler log line number ")
		b.WriteByte(byte('0' + i%10))
		b.WriteString(" with enough text to push past the stash threshold here\n")
	}
	// Put the critical fact in the middle, where the head/tail preview elides it.
	logText := b.String()
	mid := len(logText) / 2
	logText = logText[:mid] + "CRITICAL_SECRET_TOKEN=xyz789\n" + logText[mid:]

	f := mk("log", "journalctl -u svc", logText, "CRITICAL_SECRET_TOKEN=xyz789")
	f.Options.Reversible = true
	rep := Run([]Fixture{f})

	r := rep.Results[0]
	if r.Transform != "reversible" {
		t.Fatalf("expected reversible transform, got %q", r.Transform)
	}
	if r.Recoverable != 1 {
		t.Error("middle fact must remain recoverable via the stash")
	}
	if r.Inline != 0 {
		t.Error("a fact elided into the stash should not count as inline")
	}
	if !rep.Pass {
		t.Error("reversible run must PASS — recoverable, not lost")
	}
}

func TestRunDedupFactStaysInline(t *testing.T) {
	dup := strings.Repeat("key: super-important-value\n", 30)
	f := Fixture{
		Name: "dup", Command: "cat x.yaml",
		Prior: dup, Output: dup,
		MustKeep: []string{"super-important-value"},
	}
	f.Options.Dedup = true
	rep := Run([]Fixture{f})

	r := rep.Results[0]
	if r.Transform != "dedup" {
		t.Fatalf("expected dedup transform, got %q", r.Transform)
	}
	// The verbatim earlier copy is in the prompt, so the fact is inline (no
	// retrieval needed) even though the eligible block became a back-reference.
	if r.Inline != 1 || r.Recoverable != 1 {
		t.Errorf("dedup fact should be inline & recoverable, got inline=%d recover=%d", r.Inline, r.Recoverable)
	}
}

// TestRunDetectsLostFact is the most important test: it proves the gate
// actually fails when a fact is unreachable. A must_keep string that appears
// nowhere stands in for a hypothetical dropped fact.
func TestRunDetectsLostFact(t *testing.T) {
	f := mk("git", "git status",
		"On branch main\n\tmodified:   foo.go\n",
		"main", "THIS_FACT_WAS_DROPPED")
	rep := Run([]Fixture{f})
	if rep.Pass {
		t.Fatal("harness must FAIL when a declared fact is unreachable")
	}
	r := rep.Results[0]
	if r.Lost() != 1 {
		t.Errorf("expected exactly 1 lost fact, got %d", r.Lost())
	}
}

func TestLoadDefaultCorpusPasses(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	fixtures, err := LoadDefault()
	if err != nil {
		t.Fatalf("LoadDefault: %v", err)
	}
	if len(fixtures) == 0 {
		t.Fatal("built-in corpus is empty")
	}
	rep := Run(fixtures)
	if !rep.Pass {
		t.Errorf("built-in corpus must pass; %d/%d facts recoverable",
			rep.TotalRecover, rep.TotalFacts)
	}
}
