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

	// xhci_hcd should have the FL1100 quirk
	quirks = FindByModule("xhci_hcd")
	if len(quirks) != 1 {
		t.Fatalf("expected 1 quirk for xhci_hcd, got %d", len(quirks))
	}
	if quirks[0].ID != "fl1100-usb3-firmware" {
		t.Errorf("expected fl1100 quirk, got %s", quirks[0].ID)
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
	if len(all) != 7 {
		t.Errorf("expected 7 quirks in database, got %d", len(all))
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