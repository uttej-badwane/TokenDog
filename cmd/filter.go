package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"tokendog/internal/filter"
	"tokendog/internal/hook"
)

var filterCmd = &cobra.Command{
	Use:   "filter",
	Short: "Filter management — scaffolding and registry inspection",
}

var filterInitCmd = &cobra.Command{
	Use:   "init <binary-name>",
	Short: "Scaffold a new filter (creates filter source, test, and registry entry)",
	Long: `Generate the boilerplate for a new filter so contributors don't have to
hunt through the repo to figure out where everything lives.

This must be run from the TokenDog repo root. It will:
  1. Create internal/filter/<name>.go with a stub Filter signature
  2. Create internal/filter/<name>_test.go with a passthrough + lossless test
  3. Print the one-line addition needed in registrations.go (manual step)
  4. Print the one-line addition needed in internal/hook/hook.go Supported map

After running this, edit the stub to implement your strategy, run the
tests, and open a PR.`,
	Args: cobra.ExactArgs(1),
	RunE: runFilterInit,
}

var filterListCmd = &cobra.Command{
	Use:   "list",
	Short: "Print every registered filter and the binary it claims",
	RunE: func(_ *cobra.Command, _ []string) error {
		registered := filter.Registered()
		for _, b := range registered {
			fmt.Println(b)
		}
		fmt.Fprintf(os.Stderr, "\n%d filters registered.\n", len(registered))
		return nil
	},
}

func init() {
	filterCmd.AddCommand(filterInitCmd)
	filterCmd.AddCommand(filterListCmd)
}

func runFilterInit(_ *cobra.Command, args []string) error {
	name := strings.ToLower(strings.TrimSpace(args[0]))
	if name == "" {
		return fmt.Errorf("filter name required")
	}
	// Sanity: only [a-z0-9_-]+ — these become Go identifiers and CLI flags.
	for _, r := range name {
		ok := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '-'
		if !ok {
			return fmt.Errorf("filter name must match [a-z0-9_-]+, got %q", name)
		}
	}
	if _, exists := filter.Lookup(name); exists {
		return fmt.Errorf("filter %q is already registered", name)
	}
	if _, exists := hook.Supported[name]; exists {
		return fmt.Errorf("binary %q is already in hook.Supported", name)
	}

	repoRoot, err := findRepoRoot()
	if err != nil {
		return fmt.Errorf("could not locate TokenDog repo root: %w (run from within the repo)", err)
	}

	srcPath := filepath.Join(repoRoot, "internal", "filter", name+".go")
	testPath := filepath.Join(repoRoot, "internal", "filter", name+"_test.go")
	if _, err := os.Stat(srcPath); err == nil {
		return fmt.Errorf("file already exists: %s", srcPath)
	}

	titleName := strings.ToUpper(name[:1]) + name[1:]

	src := scaffoldFilterSource(name, titleName)
	test := scaffoldFilterTest(name, titleName)

	if err := os.WriteFile(srcPath, []byte(src), 0644); err != nil {
		return err
	}
	if err := os.WriteFile(testPath, []byte(test), 0644); err != nil {
		return err
	}

	fmt.Printf("Created %s\n", srcPath)
	fmt.Printf("Created %s\n", testPath)
	fmt.Printf("\nNext: add ONE line to internal/filter/registrations.go inside init():\n\n")
	fmt.Printf("    Register(%q, %sAdapter)\n\n", name, name)
	fmt.Printf("Then add ONE line to internal/hook/hook.go's Supported map:\n\n")
	fmt.Printf("    %q: %q,\n\n", name, name)
	fmt.Printf("Then implement the filter, run `go test ./internal/filter/`, and open a PR.\n")
	return nil
}

func findRepoRoot() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	dir := cwd
	for {
		// go.mod that declares "module tokendog" identifies the repo root.
		mod, err := os.ReadFile(filepath.Join(dir, "go.mod"))
		if err == nil && strings.Contains(string(mod), "module tokendog") {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("no tokendog go.mod found above %s", cwd)
		}
		dir = parent
	}
}

func scaffoldFilterSource(name, title string) string {
	return `package filter

// ` + title + ` compresses ` + name + ` output. Replace this comment with the
// strategy you're using ("strip noise X", "compact JSON Y", etc.).
//
// LOSSLESS CONTRACT: output bytes <= input bytes. The Guard wrapper enforces
// this at the cmd-layer, but filters should also pass through unchanged when
// they can't compress. See CONTRIBUTING.md for the full contract.
func ` + title + `(args []string, raw string) string {
	// TODO: implement.
	// While developing, leave this as passthrough — every test passes
	// trivially against an unchanged-output filter.
	return raw
}

// ` + name + `Adapter wires ` + title + ` into the filter registry. If your
// filter doesn't need args, leave this as the trivial form. If it needs
// subcommand routing, look at gitAdapter / dockerAdapter for the pattern.
func ` + name + `Adapter(args []string, raw string) string {
	return ` + title + `(args, raw)
}
`
}

func scaffoldFilterTest(name, title string) string {
	return `package filter

import "testing"

// TestPassthroughOnEmpty is the freebie: every filter must return empty
// for empty input. Keeps you honest while you implement.
func Test` + title + `PassthroughOnEmpty(t *testing.T) {
	if got := ` + title + `(nil, ""); got != "" {
		t.Errorf("` + title + `(empty) = %q, want empty", got)
	}
}

// TestLosslessContract — output bytes must never exceed input bytes. This
// is the universal filter invariant and Guard enforces it at the wrapper,
// but filters should be self-consistent.
func Test` + title + `LosslessContract(t *testing.T) {
	in := "sample raw output\nfor your filter\nto compress\n"
	out := ` + title + `(nil, in)
	if len(out) > len(in) {
		t.Errorf("` + title + ` inflated: %d -> %d bytes\nout: %q", len(in), len(out), out)
	}
}

// TODO: add a golden-file test with realistic ` + name + ` output.
//   - Pick a real ` + name + ` invocation that produces a representative output.
//   - Hardcode that output as ` + "`in`" + `.
//   - Assert which bytes must be preserved (lossless) and that ` + "`out`" + ` is shorter.
//   - See gh_test.go / cloud_test.go for examples.
`
}
