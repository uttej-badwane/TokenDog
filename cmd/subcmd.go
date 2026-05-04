package cmd

import "strings"

// extractSubcommand returns the first positional argument from args, ignoring
// flags and the values consumed by flags that take a separately-tokenized
// argument (e.g. `git -C path status` -> "status", not "path").
//
// `valueFlags` lists flags that consume the next arg. The `--name=value`
// form is always self-contained and never consumes the next arg, regardless
// of whether the flag is in the set.
func extractSubcommand(args []string, valueFlags map[string]bool) string {
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

// Per-tool value-flag sets. We list only the flags that change which
// subcommand executes if their argument is misclassified — global options
// like `--verbose` (boolean) don't need to be here.

var gitValueFlags = map[string]bool{
	"-C": true, "-c": true,
	"--git-dir": true, "--work-tree": true, "--namespace": true,
	"--exec-path": true, "--config-env": true,
}

var ghValueFlags = map[string]bool{
	"-R": true, "--repo": true,
	"--hostname": true,
}

var dockerValueFlags = map[string]bool{
	"-H": true, "--host": true,
	"-l": true, "--log-level": true,
	"--config": true, "--context": true,
	"--tlscacert": true, "--tlscert": true, "--tlskey": true,
}

var kubectlValueFlags = map[string]bool{
	"-n": true, "--namespace": true,
	"--context": true, "--cluster": true, "--user": true,
	"--kubeconfig": true,
	"--as":         true, "--as-group": true,
	"--token": true,
	"-s":      true, "--server": true,
	"--request-timeout": true, "--cache-dir": true,
}

var cargoValueFlags = map[string]bool{
	"--manifest-path": true, "--target-dir": true, "--color": true,
	"-Z": true, "--config": true,
}
