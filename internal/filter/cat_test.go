package filter

import (
	"strings"
	"testing"
)

func TestCat_collapsesBlanks(t *testing.T) {
	input := "line1\n\n\n\n\nline2\n"
	out := Cat(input)
	// At most 2 consecutive blank lines should remain (= 3 newlines max between content).
	if strings.Contains(out, "\n\n\n\n") {
		t.Errorf("3+ consecutive blank lines not collapsed: %q", out)
	}
	if !strings.Contains(out, "line1") || !strings.Contains(out, "line2") {
		t.Errorf("content was dropped: %q", out)
	}
}

func TestCat_stripsTrailingWhitespace(t *testing.T) {
	input := "func foo() {   \n    return 42   \n}\n"
	out := Cat(input)
	for _, line := range strings.Split(out, "\n") {
		if strings.TrimRight(line, " \t") != line {
			t.Errorf("trailing whitespace not stripped in line: %q", line)
		}
	}
	if !strings.Contains(out, "return 42") {
		t.Error("content was dropped")
	}
}

func TestCat_preservesCode(t *testing.T) {
	input := "package main\n\nfunc main() {\n\tfmt.Println(\"hello\")\n}\n"
	out := Cat(input)
	// Code structure must be intact.
	if !strings.Contains(out, "func main()") || !strings.Contains(out, "fmt.Println") {
		t.Errorf("code was modified: %q", out)
	}
}

func TestCat_neverExpands(t *testing.T) {
	inputs := []string{
		"small file\n",
		"",
		"no blank lines here\n",
	}
	for _, in := range inputs {
		out := Cat(in)
		if len(out) > len(in) {
			t.Errorf("output longer than input for %q", in)
		}
	}
}
