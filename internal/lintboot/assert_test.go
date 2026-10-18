package lintboot

import (
	"strings"
	"testing"

	"github.com/eonicman/pinscher/internal/probe"
)

func TestParseAssertion(t *testing.T) {
	field, device, value, err := ParseAssertion("driver:0000:02:00.0=vfio-pci")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if field != "driver" {
		t.Errorf("field = %q, want %q", field, "driver")
	}
	if device != "0000:02:00.0" {
		t.Errorf("device = %q, want %q", device, "0000:02:00.0")
	}
	if value != "vfio-pci" {
		t.Errorf("value = %q, want %q", value, "vfio-pci")
	}
}

// vendor:device-ID queries contain both ':' and no '=' — this is the case
// that would break a naive split-on-last-':' approach.
func TestParseAssertionVendorDeviceIDQuery(t *testing.T) {
	field, device, value, err := ParseAssertion("driver:1002:6798=amdgpu")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if field != "driver" || device != "1002:6798" || value != "amdgpu" {
		t.Errorf("got field=%q device=%q value=%q", field, device, value)
	}
}

func TestParseAssertionMissingColon(t *testing.T) {
	if _, _, _, err := ParseAssertion("driver=amdgpu"); err == nil {
		t.Error("expected an error for a missing ':', got nil")
	}
}

func TestParseAssertionMissingEquals(t *testing.T) {
	if _, _, _, err := ParseAssertion("driver:amdgpu"); err == nil {
		t.Error("expected an error for a missing '=', got nil")
	}
}

func TestParseAssertionUnknownField(t *testing.T) {
	_, _, _, err := ParseAssertion("nonsense_field:amdgpu=1")
	if err == nil {
		t.Error("expected an error for an unknown field, got nil")
	}
}

func TestParseAssertionEmptyDevice(t *testing.T) {
	if _, _, _, err := ParseAssertion("driver:=amdgpu"); err == nil {
		t.Error("expected an error for an empty device query, got nil")
	}
}

func TestCheckAssertionsEmpty(t *testing.T) {
	results, err := CheckAssertions(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if results != nil {
		t.Errorf("expected nil results for no assertions, got %v", results)
	}
}

func TestCheckAssertionsAgainstRealHardware(t *testing.T) {
	// This VM/host must have at least one device with a loaded driver for
	// this test to mean anything; skip gracefully if not.
	probeResult, err := probe.Probe()
	if err != nil {
		t.Fatalf("probe failed: %v", err)
	}
	var realDevice, realDriver string
	for _, d := range probeResult.Devices {
		// PCI devices only: ACPI/platform paths have other shapes (found on a
		// real Ubuntu box where the first driver-bound device was ACPI).
		if d.Driver != "" && strings.HasPrefix(d.Path, "/sys/bus/pci/devices/") {
			realDevice = strings.TrimPrefix(d.Path, "/sys/bus/pci/devices/")
			realDriver = d.Driver
			break
		}
	}
	if realDriver == "" {
		t.Skip("no device with a loaded driver found on this host")
	}

	t.Run("passing assertion", func(t *testing.T) {
		results, err := CheckAssertions([]string{"driver:" + realDevice + "=" + realDriver})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(results) != 1 {
			t.Fatalf("expected 1 result, got %d", len(results))
		}
		if !results[0].Pass {
			t.Errorf("expected assertion to pass, got Actual=%q Error=%q", results[0].Actual, results[0].Error)
		}
	})

	t.Run("failing assertion", func(t *testing.T) {
		results, err := CheckAssertions([]string{"driver:" + realDevice + "=DEFINITELY_NOT_THE_REAL_DRIVER"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(results) != 1 {
			t.Fatalf("expected 1 result, got %d", len(results))
		}
		if results[0].Pass {
			t.Error("expected assertion to fail, got Pass=true")
		}
		if results[0].Actual != realDriver {
			t.Errorf("Actual = %q, want %q", results[0].Actual, realDriver)
		}
	})

	t.Run("device not found", func(t *testing.T) {
		results, err := CheckAssertions([]string{"driver:totally_fake_device_xyz=anything"})
		if err != nil {
			t.Fatalf("expected a per-result error, not a hard error: %v", err)
		}
		if len(results) != 1 {
			t.Fatalf("expected 1 result, got %d", len(results))
		}
		if results[0].Pass {
			t.Error("expected Pass=false for a device that doesn't exist")
		}
		if results[0].Error == "" {
			t.Error("expected Error to be set for a device-not-found result")
		}
	})

	t.Run("malformed assertion is a hard error, not a result", func(t *testing.T) {
		_, err := CheckAssertions([]string{"driver:" + realDevice}) // missing =value
		if err == nil {
			t.Error("expected a hard error for malformed syntax, got nil")
		}
	})

	t.Run("one bad assertion does not hide a later good one's hard-error check", func(t *testing.T) {
		// A malformed assertion anywhere in the batch is a hard error for the
		// whole call (it's a typo, not data) — confirm this holds even when
		// a later assertion in the same batch would have been fine.
		_, err := CheckAssertions([]string{
			"driver:" + realDevice + "=" + realDriver,
			"bad syntax no colon or equals",
		})
		if err == nil {
			t.Error("expected a hard error when any assertion in the batch is malformed")
		}
	})
}
