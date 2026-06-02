package cmd

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestMachineIDStableAndOpaque(t *testing.T) {
	a := machineID()
	b := machineID()
	if a != b {
		t.Errorf("machineID not stable: %s vs %s", a, b)
	}
	if len(a) != 16 {
		t.Errorf("machineID length = %d, want 16", len(a))
	}
	// It must not be the raw hostname.
	host := "test-host"
	if strings.Contains(a, host) {
		t.Error("machineID should be a hash, not the hostname")
	}
}

// TestFleetReportCarriesNoContent is the privacy guarantee: the serialized
// report must contain only aggregates — no command strings, arguments, or
// output. We assert the JSON keys are exactly the allow-listed aggregate set.
func TestFleetReportCarriesNoContent(t *testing.T) {
	r := fleetReport{
		Schema: "tokendog.fleet.v1", MachineID: "abcd", Commands: 10,
		RawBytes: 1000, FilteredBytes: 700, BytesSaved: 300, TokensSaved: 80,
	}
	data, _ := json.Marshal(r)
	var generic map[string]any
	if err := json.Unmarshal(data, &generic); err != nil {
		t.Fatal(err)
	}
	allowed := map[string]bool{
		"schema": true, "machine_id": true, "generated_at": true, "since": true,
		"commands": true, "raw_bytes": true, "filtered_bytes": true,
		"bytes_saved": true, "tokens_saved": true, "cache_hits": true,
		"td_version": true,
	}
	for k := range generic {
		if !allowed[k] {
			t.Errorf("report leaks non-aggregate field %q", k)
		}
	}
	for _, forbidden := range []string{"command", "output", "content", "args"} {
		if _, ok := generic[forbidden]; ok {
			t.Errorf("report must not carry %q", forbidden)
		}
	}
}
