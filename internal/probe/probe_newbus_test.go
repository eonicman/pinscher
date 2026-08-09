package probe

import (
	"testing"
)

func TestProbeBluetooth(t *testing.T) {
	// Test that Bluetooth probe runs without errors
	// May return empty on systems without Bluetooth
	devices, err := probeBluetooth()
	if err != nil {
		t.Fatalf("probeBluetooth() error: %v", err)
	}

	for _, dev := range devices {
		if dev.Bus != "bluetooth" {
			t.Errorf("expected bus 'bluetooth', got '%s'", dev.Bus)
		}
		if dev.Path == "" {
			t.Error("expected non-empty path")
		}
		// Bluetooth devices should have capabilities
		if dev.Capabilities != nil {
			if _, ok := dev.Capabilities["type"]; !ok {
				t.Error("expected 'type' in bluetooth capabilities")
			}
		}
	}
}

func TestProbeWiFi(t *testing.T) {
	// Test that WiFi probe runs without errors
	// May return empty on systems without WiFi
	devices, err := probeWiFi()
	if err != nil {
		t.Fatalf("probeWiFi() error: %v", err)
	}

	for _, dev := range devices {
		if dev.Bus != "wifi" {
			t.Errorf("expected bus 'wifi', got '%s'", dev.Bus)
		}
		if dev.Path == "" {
			t.Error("expected non-empty path")
		}
		// WiFi devices should have capabilities
		if dev.Capabilities == nil {
			t.Error("expected non-nil capabilities for WiFi device")
		}
	}
}

func TestProbePlatform(t *testing.T) {
	// Test that platform probe returns devices
	devices, err := probePlatform()
	if err != nil {
		t.Fatalf("probePlatform() error: %v", err)
	}

	if len(devices) == 0 {
		t.Error("expected at least one platform device")
	}

	for _, dev := range devices {
		if dev.Bus != "platform" {
			t.Errorf("expected bus 'platform', got '%s'", dev.Bus)
		}
		if dev.Path == "" {
			t.Error("expected non-empty path")
		}
		// Platform devices should have a description (name or entry name)
		if dev.Description == "" {
			t.Error("expected non-empty description for platform device")
		}
		// Check modalias format (platform:xxx or acpi:xxx)
		if dev.Modalias != "" {
			if dev.Modalias[0:8] != "platform" && dev.Modalias[0:4] != "acpi" {
				t.Errorf("unexpected modalias format: %s", dev.Modalias)
			}
		}
	}
}

func TestProbeACPI(t *testing.T) {
	// Test that ACPI probe returns devices (but filters device:* containers)
	devices, err := probeACPI()
	if err != nil {
		t.Fatalf("probeACPI() error: %v", err)
	}

	for _, dev := range devices {
		if dev.Bus != "acpi" {
			t.Errorf("expected bus 'acpi', got '%s'", dev.Bus)
		}
		if dev.Path == "" {
			t.Error("expected non-empty path")
		}
		// ACPI device:* entries should be filtered out
		if len(dev.Description) >= 7 && dev.Description[:7] == "device:" {
			t.Errorf("ACPI container device: should be filtered: %s", dev.Description)
		}
	}
}

func TestClassNameForPCI(t *testing.T) {
	tests := []struct {
		class string
		want  string
	}{
		{"030000", "VGA Compatible Controller"},
		{"0300", "VGA Compatible Controller"},
		{"03", "Display Controller"},
		{"020000", "Ethernet Controller"},
		{"0280", "Network Controller (Wireless)"},
		{"0c0330", "USB Controller"},
		{"0c03", "USB Controller"},
		{"0106", "SATA Controller"},
		{"010802", "NVMe Controller"},
		{"ff", "Other"},
		{"", ""},
		{"9999", ""},
	}

	for _, tt := range tests {
		got := classNameForPCI(tt.class)
		if got != tt.want {
			t.Errorf("classNameForPCI(%q) = %q, want %q", tt.class, got, tt.want)
		}
	}
}

func TestClassNameForUSB(t *testing.T) {
	tests := []struct {
		class string
		want  string
	}{
		{"03", "Human Interface Device (HID)"},
		{"08", "Mass Storage"},
		{"09", "Hub"},
		{"e0", "Wireless Controller"},
		{"ff", "Vendor Specific"},
		{"", ""},
	}

	for _, tt := range tests {
		got := classNameForUSB(tt.class)
		if got != tt.want {
			t.Errorf("classNameForUSB(%q) = %q, want %q", tt.class, got, tt.want)
		}
	}
}

func TestProbeWithNewBuses(t *testing.T) {
	// Full probe should include new bus types
	result, err := Probe()
	if err != nil {
		t.Fatalf("Probe() error: %v", err)
	}

	// Should have at least PCI and USB devices
	if result.Summary.ByBus["pci"] == 0 {
		t.Error("expected at least one PCI device")
	}
	if result.Summary.ByBus["usb"] == 0 {
		t.Error("expected at least one USB device")
	}
	// Should have platform devices
	if result.Summary.ByBus["platform"] == 0 {
		t.Error("expected at least one platform device")
	}
	// Should have ACPI devices (but fewer than before due to filtering)
	if result.Summary.ByBus["acpi"] == 0 {
		t.Error("expected at least one ACPI device")
	}

	// Total should be reasonable (not 590 with all device:* containers)
	if result.Summary.Total > 500 {
		t.Errorf("too many devices (%d) — ACPI device:* containers may not be filtered", result.Summary.Total)
	}

	// All devices should have a bus type
	for _, dev := range result.Devices {
		if dev.Bus == "" {
			t.Errorf("device %s has empty bus", dev.Path)
		}
	}
}