package filter

// init registers every filter the package ships. Adding a new filter:
//  1. Implement it in its own file (e.g. grep.go) with whatever signature
//     makes sense for that tool.
//  2. Add ONE line below — `Register("grep", grepAdapter)` — pointing at a
//     small adapter that converts (args, raw) into your filter's natural
//     signature (subcommand-extraction, runner-name picking, etc.).
//  3. Add the binary to internal/hook.Supported so the hook rewrites it.
//
// That's it. cmd/run.go and cmd/replay.go consume this registry — they
// don't need to know about new filters.
func init() {
	Register("git", gitAdapter)
	Register("ls", lsAdapter)
	Register("find", findAdapter)
	Register("docker", dockerAdapter)
	Register("jq", jqAdapter)
	Register("curl", curlAdapter)
	Register("kubectl", kubectlAdapter)
	Register("gh", ghAdapter)
	Register("pytest", pytestAdapter)
	Register("jest", jestAdapter)
	Register("vitest", vitestAdapter)
	Register("go", goAdapter)
	Register("cargo", cargoAdapter)
	Register("npm", pkgAdapter)
	Register("pnpm", pkgAdapter)
	Register("yarn", pkgAdapter)
	Register("pip", pkgAdapter)
	Register("aws", cloudAdapter)
	Register("gcloud", cloudAdapter)
	Register("az", cloudAdapter)
	Register("make", makeAdapter)
	Register("grep", grepAdapter)
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

// Adapters convert each filter's natural signature into the registry's
// uniform Filter type (args, raw → filtered). Most are one-liners; the
// ones with subcommand routing peel off the subcmd first.

func gitAdapter(args []string, raw string) string {
	return Git(ExtractSubcmd(args, gitValueFlags), raw)
}
func ghAdapter(args []string, raw string) string {
	return GH(ExtractSubcmd(args, ghValueFlags), raw)
}
func dockerAdapter(args []string, raw string) string {
	return Docker(ExtractSubcmd(args, dockerValueFlags), raw)
}
func kubectlAdapter(args []string, raw string) string {
	return Kubectl(ExtractSubcmd(args, kubectlValueFlags), raw)
}

func lsAdapter(_ []string, raw string) string    { return Ls(raw) }
func findAdapter(_ []string, raw string) string  { return Find(raw) }
func jqAdapter(_ []string, raw string) string    { return JQ(raw) }
func curlAdapter(_ []string, raw string) string  { return Curl(raw) }
func makeAdapter(_ []string, raw string) string  { return Make(raw) }
func pkgAdapter(_ []string, raw string) string   { return PackageManager(raw) }
func cloudAdapter(_ []string, raw string) string { return Cloud(raw) }

func pytestAdapter(_ []string, raw string) string { return Test("pytest", raw) }
func jestAdapter(_ []string, raw string) string   { return Test("jest", raw) }
func vitestAdapter(_ []string, raw string) string { return Test("vitest", raw) }

// goAdapter only filters `go test` output. Other go subcommands (build,
// run, vet, etc.) emit user-relevant content (compiler errors, program
// output) that mustn't be touched.
func goAdapter(args []string, raw string) string {
	if FirstNonFlag(args) != "test" {
		return raw
	}
	return Test("go", raw)
}

// cargoAdapter routes by subcommand — test gets the test runner, build/
// check/fetch/update get the package-manager noise stripper, everything
// else passes through.
func cargoAdapter(args []string, raw string) string {
	switch ExtractSubcmd(args, cargoValueFlags) {
	case "test":
		return Test("cargo", raw)
	case "build", "check", "fetch", "update":
		return PackageManager(raw)
	}
	return raw
}
