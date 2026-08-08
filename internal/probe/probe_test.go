package probe

import (
	"testing"
)

func TestStripHex(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"0x1002", "1002"},
		{"1002", "1002"},
		{"0x0c0330", "0c0330"},
		{"", ""},
	}
	for _, tt := range tests {
		got := stripHex(tt.input)
		if got != tt.want {
			t.Errorf("stripHex(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestProbe(t *testing.T) {
	// This test runs on the actual system, so just verify it doesn't error
	result, err := Probe()
	if err != nil {
		t.Fatalf("Probe() failed: %v", err)
	}
	if result.Hostname == "" {
		t.Error("expected non-empty hostname")
	}
	if result.Kernel == "" {
		t.Error("expected non-empty kernel version")
	}
	if result.Summary.Total == 0 {
		t.Error("expected at least one device")
	}
	if result.Summary.ByBus == nil {
		t.Error("expected non-nil ByBus map")
	}
}

func TestProbeJSON(t *testing.T) {
	result, err := Probe()
	if err != nil {
		t.Fatalf("Probe() failed: %v", err)
	}
	json, err := result.JSON()
	if err != nil {
		t.Fatalf("JSON() failed: %v", err)
	}
	if json == "" {
		t.Error("expected non-empty JSON output")
	}
	// Verify JSON is valid by checking it starts with {
	if json[0] != '{' {
		t.Errorf("JSON output doesn't start with {: %s", json[:20])
	}
}