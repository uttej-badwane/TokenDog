package harness

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"tokendog/internal/redact"
)

// scanSecrets runs the shared redact patterns over a config file,
// line by line so findings carry a line number. The finding text names
// the pattern and the location — never the value. PEM blocks span lines,
// so they get a whole-content pass.
func scanSecrets(path, scope string, content []byte) []Finding {
	if !utf8.Valid(content) {
		return nil
	}
	var out []Finding
	text := string(content)
	for i, line := range strings.Split(text, "\n") {
		for _, name := range redact.FindNames(line) {
			if name == "pem-block" {
				continue // handled below on the full content
			}
			out = append(out, Finding{
				File: path, Scope: scope, Dimension: "secrets", Severity: SeverityCritical,
				Issue: fmt.Sprintf("possible %s on line %d", name, i+1),
				Fix:   "move it to an env var or credential helper, scrub the file, and rotate the secret",
			})
		}
	}
	if strings.Contains(text, "-----BEGIN ") {
		for _, name := range redact.FindNames(text) {
			if name == "pem-block" {
				out = append(out, Finding{
					File: path, Scope: scope, Dimension: "secrets", Severity: SeverityCritical,
					Issue: "contains private key material (PEM block)",
					Fix:   "remove the key from the config and rotate it",
				})
			}
		}
	}
	return out
}
