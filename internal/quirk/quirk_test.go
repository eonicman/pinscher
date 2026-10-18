package quirk

import "testing"

func TestFindByDevice(t *testing.T) {
	// AMD FirePro D700 (Tahiti XT)
	quirks := FindByDevice("1002", "6799")
	if len(quirks) != 2 {
		t.Fatalf("expected 2 quirks for 1002:6799, got %d", len(quirks))
	}

	// Check that both si_support and dc quirks are found
	foundSI, foundDC := false, false
	for _, q := range quirks {
		if q.ID == "amdgpu-tahiti-xt-si-support" {
			foundSI = true
			if q.Params["si_support"] != "1" {
				t.Errorf("expected si_support=1, got %s", q.Params["si_support"])
			}
		}
		if q.ID == "amdgpu-tahiti-xt-dc" {
			foundDC = true
			if q.Params["dc"] != "1" {
				t.Errorf("expected dc=1, got %s", q.Params["dc"])
			}
		}
	}
	if !foundSI {
		t.Error("expected to find si_support quirk")
	}
	if !foundDC {
		t.Error("expected to find dc quirk")
	}
}

func TestFindByModule(t *testing.T) {
	quirks := FindByModule("amdgpu")
	if len(quirks) < 2 {
		t.Fatalf("expected at least 2 quirks for amdgpu, got %d", len(quirks))
	}

	// xhci_hcd should have the FL1100 quirk (and potentially others)
	quirks = FindByModule("xhci_hcd")
	if len(quirks) < 1 {
		t.Fatalf("expected at least 1 quirk for xhci_hcd, got %d", len(quirks))
	}

	// Verify the FL1100 quirk is present
	foundFL1100 := false
	for _, q := range quirks {
		if q.ID == "fl1100-usb3-firmware" {
			foundFL1100 = true
			break
		}
	}
	if !foundFL1100 {
		t.Error("expected to find FL1100 quirk for xhci_hcd")
	}
}

func TestFindByDeviceNotFound(t *testing.T) {
	quirks := FindByDevice("0000", "0000")
	if len(quirks) != 0 {
		t.Errorf("expected no quirks for unknown device, got %d", len(quirks))
	}
}

func TestFindAll(t *testing.T) {
	all := FindAll()
	minExpected := 20 // Expanded from original 7
	if len(all) < minExpected {
		t.Errorf("expected at least %d quirks in database, got %d", minExpected, len(all))
	}
}

func TestQuirkFields(t *testing.T) {
	q := FindByModule("amdgpu")[0]
	if q.Module == "" {
		t.Error("expected non-empty Module")
	}
	if q.Severity == "" {
		t.Error("expected non-empty Severity")
	}
	if q.Citation == "" {
		t.Error("expected non-empty Citation")
	}
	if q.Category == "" {
		t.Error("expected non-empty Category")
	}
}

func TestNewQuirks(t *testing.T) {
	// Test ThinkPad quirk
	quirks := FindByModule("psmouse")
	if len(quirks) == 0 {
		t.Error("expected psmouse quirks (ThinkPad trackpoint)")
	}

	// Test Framework audio quirk
	quirks = FindByModule("snd_hda_intel")
	if len(quirks) < 2 {
		t.Errorf("expected at least 2 quirks for snd_hda_intel, got %d", len(quirks))
	}

	// Test NVIDIA quirk
	quirks = FindByModule("nvidia")
	if len(quirks) == 0 {
		t.Error("expected nvidia quirks")
	}

	// Test iwlwifi quirk
	quirks = FindByModule("iwlwifi")
	if len(quirks) == 0 {
		t.Error("expected iwlwifi quirks (Intel AX WiFi)")
	}

	// Test r8169 quirk
	quirks = FindByModule("r8169")
	if len(quirks) == 0 {
		t.Error("expected r8169 quirks (Realtek RTL8125)")
	}
}

func TestQuirkCategories(t *testing.T) {
	categories := map[string]bool{
		"gpu":         true,
		"usb":         true,
		"storage":     true,
		"audio":       true,
		"network":     true,
		"apple":       true,
		"power":       true,
		"input":       true,
		"thunderbolt": true,
	}

	all := FindAll()
	foundCategories := make(map[string]int)
	for _, q := range all {
		foundCategories[q.Category]++
	}

	for cat := range categories {
		if foundCategories[cat] == 0 {
			t.Errorf("expected at least one quirk in category %s", cat)
		}
	}
}
