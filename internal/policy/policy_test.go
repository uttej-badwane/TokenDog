package policy

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSaveLoadRoundTrip(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	p := Policy{Dedup: Bool(false), Reversible: Bool(true), StashMinBytes: Int(4096)}
	if err := Save(p); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got := Load()
	if got.Dedup == nil || *got.Dedup != false {
		t.Errorf("Dedup round-trip failed: %+v", got.Dedup)
	}
	if got.Reversible == nil || *got.Reversible != true {
		t.Errorf("Reversible round-trip failed")
	}
	if got.StashMinBytes == nil || *got.StashMinBytes != 4096 {
		t.Errorf("StashMinBytes round-trip failed")
	}
}

func TestLoadMissingIsEmpty(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if p := Load(); !p.Empty() {
		t.Errorf("missing policy should be empty, got %+v", p)
	}
}

func TestLoadMalformedIsEmpty(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	path, _ := Path()
	os.MkdirAll(filepath.Dir(path), 0o755)
	os.WriteFile(path, []byte("not json"), 0o644)
	if p := Load(); !p.Empty() {
		t.Error("malformed policy must load as empty, not break")
	}
}

func TestPartialPolicy(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	// Only reversible set; dedup/stash_min left at default.
	if err := Save(Policy{Reversible: Bool(true)}); err != nil {
		t.Fatal(err)
	}
	got := Load()
	if got.Dedup != nil {
		t.Error("unset dedup should stay nil (default)")
	}
	if got.Reversible == nil || !*got.Reversible {
		t.Error("reversible should be set")
	}
}
