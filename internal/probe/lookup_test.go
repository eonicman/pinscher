package probe

import "testing"

// Added by Ghost, 2026-10-18: lspci-style short PCI addresses must match.
func TestFindDeviceShortPCIAddress(t *testing.T) {
	r := &ProbeResult{Devices: []Device{
		{Path: "/sys/bus/pci/devices/0000:00:1f.2", Driver: "ahci"},
		{Path: "/sys/bus/pci/devices/0000:01:00.0", Driver: "radeon"},
	}}
	for _, q := range []string{"01:00.0", "0000:01:00.0"} {
		d, err := FindDevice(r, q)
		if err != nil || d.Driver != "radeon" {
			t.Errorf("FindDevice(%q) = %v, %v; want radeon device", q, d, err)
		}
	}
	if _, err := FindDevice(r, "02:00.0"); err == nil {
		t.Error("expected not-found for 02:00.0")
	}
}
