package filter

// Filter applies tool-specific compression to raw stdout. raw is the
// captured output; args is the argv after the binary name (e.g. for
// `git -C /path log -3`, args is ["-C","/path","log","-3"]). The return
// is the filtered string. A filter MUST NOT inflate raw — the universal
// Guard at the wrapper layer enforces this, but filters should also
// internally pass-through unchanged when they can't help.
type Filter func(args []string, raw string) string

// registry holds the canonical binary→filter mapping. Each filter file
// registers itself via init(). cmd/run.go and cmd/replay.go both consume
// this registry, so adding a new filter means touching exactly ONE place
// (the new filter's source file) instead of five.
//
// The map is intentionally unexported; callers go through Lookup so we can
// add behavior (e.g. "no-op for unsupported binaries") in one place.
var registry = map[string]Filter{}

// Register adds a filter for a binary. Called from filter source files'
// init() functions. Panics on duplicate registration — that means two
// filters claim the same binary, which is always a bug worth catching at
// startup rather than at runtime on a user's machine.
func Register(binary string, fn Filter) {
	if _, exists := registry[binary]; exists {
		panic("filter: duplicate registration for binary " + binary)
	}
	registry[binary] = fn
}

// Lookup returns the registered filter for binary, or (nil, false) if no
// filter is registered. Callers that fall through to passthrough should
// check ok and emit raw unchanged.
func Lookup(binary string) (Filter, bool) {
	fn, ok := registry[binary]
	return fn, ok
}

// Registered returns the canonical list of registered binaries. Stable
// across calls within one process. Used by `td discover` to classify
// transcript history and by `td filter init` (the contributor scaffolder)
// to error on duplicate names before generating files.
func Registered() []string {
	out := make([]string, 0, len(registry))
	for k := range registry {
		out = append(out, k)
	}
	return out
}

// Apply is the convenience entry-point: looks up the filter, runs it with
// Guard so a misbehaving filter can't inflate output, returns the original
// raw with applied=false when no filter is registered. cmd-layer callers
// should prefer this over Lookup unless they need the registration check
// (e.g. for diagnostic output).
func Apply(binary string, args []string, raw string) (filtered string, applied bool) {
	fn, ok := Lookup(binary)
	if !ok {
		return raw, false
	}
	return Guard(raw, fn(args, raw)), true
}
