package config

import (
	"os"
	"testing"
)

func TestGetSafetyLevel(t *testing.T) {
	tests := []struct {
		module string
		param  string
		want   SafetyLevel
	}{
		{"amdgpu", "si_support", Safe},
		{"amdgpu", "dc", Safe},
		{"amdgpu", "ppfeaturemask", Warning},
		{"amdgpu", "test", Blocked},
		{"amdgpu", "unknown_param", Blocked},
		{"unknown_module", "anything", Blocked},
		{"nvme", "default_ps_max_latency_us", Warning},
		{"snd_hda_intel", "model", Safe},
	}

	for _, tt := range tests {
		got := getSafetyLevel(tt.module, tt.param)
		if got != tt.want {
			t.Errorf("getSafetyLevel(%s, %s) = %v, want %v", tt.module, tt.param, got, tt.want)
		}
	}
}

func TestAuditLog(t *testing.T) {
	// Reset audit log
	auditLog = nil

	// GetParam on a non-existent module should fail
	_, err := GetParam("nonexistent_module_xyz", "param")
	if err == nil {
		t.Error("expected error for nonexistent module param")
	}

	// PrintAuditLog with no operations
	log := PrintAuditLog()
	if log != "No operations logged.\n" {
		t.Errorf("expected empty audit log, got: %s", log)
	}
}

func TestApplyProfileFormat(t *testing.T) {
	// Test that the key=value format parser works
	// Create a temp profile file
	tmpFile, err := os.CreateTemp("", "pinscher-profile-*.conf")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())

	// Write a test profile
	content := `# Test profile for Annie (Mac Pro 6,1)
amdgpu.si_support=1
amdgpu.dc=1
# This is a comment
`
	if _, err := tmpFile.WriteString(content); err != nil {
		t.Fatal(err)
	}
	tmpFile.Close()

	// ApplyProfile should parse and attempt to set these
	// Using dry-run so we don't need root
	ops, err := ApplyProfile(tmpFile.Name(), true)
	if err != nil {
		t.Fatalf("ApplyProfile failed: %v", err)
	}
	if len(ops) != 2 {
		t.Errorf("expected 2 operations, got %d", len(ops))
	}
}