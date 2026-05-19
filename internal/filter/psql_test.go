package filter

import (
	"strings"
	"testing"
)

var psqlSample = ` id | name  | email
----+-------+--------------------
  1 | Alice | alice@example.com
  2 | Bob   | bob@example.com
(2 rows)

Time: 3.456 ms
`

func TestPsql_stripsSeparatorLines(t *testing.T) {
	out := Psql(psqlSample)
	if strings.Contains(out, "----+") {
		t.Errorf("separator line not removed: %q", out)
	}
}

func TestPsql_stripsTimingLine(t *testing.T) {
	out := Psql(psqlSample)
	if strings.Contains(out, "Time:") {
		t.Errorf("timing line not removed: %q", out)
	}
}

func TestPsql_preservesRows(t *testing.T) {
	out := Psql(psqlSample)
	for _, want := range []string{"Alice", "alice@example.com", "Bob", "(2 rows)"} {
		if !strings.Contains(out, want) {
			t.Errorf("row data %q missing from output: %q", want, out)
		}
	}
}

func TestPsql_neverExpands(t *testing.T) {
	out := Psql(psqlSample)
	if len(out) > len(psqlSample) {
		t.Errorf("output longer than input: %d > %d", len(out), len(psqlSample))
	}
}

func TestPsql_passthroughOnEmpty(t *testing.T) {
	if Psql("") != "" {
		t.Error("empty input should return empty")
	}
}

func TestIsSeparatorLine(t *testing.T) {
	cases := []struct {
		line string
		want bool
	}{
		{"----+-------+-------", true},
		{" id | name ", false},
		{"  1 | Alice ", false},
		{"(2 rows)", false},
		{"", false},
		{"---", true},
	}
	for _, tc := range cases {
		if got := isSeparatorLine(tc.line); got != tc.want {
			t.Errorf("isSeparatorLine(%q) = %v, want %v", tc.line, got, tc.want)
		}
	}
}
