package filter

import "strings"

// ExtractSubcmd returns the first positional argument from args, ignoring
// flags and the values consumed by flags that take a separately-tokenized
// argument (e.g. `git -C path status` -> "status", not "path").
//
// valueFlags lists flags that consume the next arg. The `--name=value`
// form is always self-contained and never consumes the next arg, regardless
// of whether the flag is in the set.
func ExtractSubcmd(args []string, valueFlags map[string]bool) string {
	skipNext := false
	for _, a := range args {
		if skipNext {
			skipNext = false
			continue
		}
		if !strings.HasPrefix(a, "-") {
			return a
		}
		if strings.Contains(a, "=") {
			continue
		}
		if valueFlags[a] {
			skipNext = true
		}
	}
	return ""
}

// FirstNonFlag is the simpler form: first arg that doesn't start with "-".
// Used by tools like `go` where flag arguments don't consume positional
// slots.
func FirstNonFlag(args []string) string {
	for _, a := range args {
		if !strings.HasPrefix(a, "-") {
			return a
		}
	}
	return ""
}

// SubcmdFor returns the subcommand-style first positional arg for a given
// binary, using the registered value-flag set. cmd-layer callers use this
// for analytics grouping and replay classification — it ensures live and
// replay always classify the same command identically.
//
// Binaries without a meaningful subcommand concept (find, ls, jq, curl,
// make, pkg managers, cloud CLIs) return "" so the analytics key is just
// the binary name. Unregistered binaries fall back to FirstNonFlag.
func SubcmdFor(binary string, args []string) string {
	switch binary {
	case "git":
		return ExtractSubcmd(args, gitValueFlags)
	case "gh":
		return ExtractSubcmd(args, ghValueFlags)
	case "docker":
		return ExtractSubcmd(args, dockerValueFlags)
	case "kubectl":
		return ExtractSubcmd(args, kubectlValueFlags)
	case "cargo":
		return ExtractSubcmd(args, cargoValueFlags)
	case "go":
		return FirstNonFlag(args)
	}
	return ""
}
