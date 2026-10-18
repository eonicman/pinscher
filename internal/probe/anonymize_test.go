package probe

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestMaskMACs(t *testing.T) {
	cases := map[string]string{
		"4c:74:bf:12:34:56":      "4c:74:bf:xx:xx:xx",
		"4C-74-BF-12-34-56":      "4C-74-BF-xx-xx-xx",
		"addr 4c:74:bf:12:34:56": "addr 4c:74:bf:xx:xx:xx",
		// not MACs: must be left alone
		"0000:01:00.0":                 "0000:01:00.0",
		"01:00.0":                      "01:00.0",
		"pci:v000014E4d000043A0sv0000": "pci:v000014E4d000043A0sv0000",
		"1002:6799":                    "1002:6799",
		"":                             "",
	}
	for in, want := range cases {
		if got := maskMACs(in); got != want {
			t.Errorf("maskMACs(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestAnonymize(t *testing.T) {
	r := &ProbeResult{
		Hostname: "my-secret-box",
		Kernel:   "7.0.0-generic",
		Devices: []Device{
			{
				Bus: "wifi", Path: "/sys/class/ieee80211/phy0", Vendor: "14e4", DeviceID: "43a0",
				Description: "BCM4360 802.11ac", Driver: "wl",
				Capabilities: map[string]string{
					"mac_address": "4c:74:bf:12:34:56", "ht_capable": "1", "serial_number": "ABC123",
				},
			},
			{Bus: "pci", Path: "0000:01:00.0", Vendor: "1002", DeviceID: "6799", Driver: "amdgpu"},
		},
	}
	r.Anonymize()

	if r.Hostname != AnonymizedHostname {
		t.Errorf("hostname = %q, want %q", r.Hostname, AnonymizedHostname)
	}
	w := r.Devices[0]
	if got := w.Capabilities["mac_address"]; got != "4c:74:bf:xx:xx:xx" {
		t.Errorf("mac_address = %q, want vendor prefix only", got)
	}
	if got := w.Capabilities["serial_number"]; got != "redacted" {
		t.Errorf("serial_number = %q, want redacted", got)
	}
	// Useful hardware data must survive.
	if w.Vendor != "14e4" || w.DeviceID != "43a0" || w.Driver != "wl" || w.Capabilities["ht_capable"] != "1" {
		t.Errorf("hardware data was altered: %+v", w)
	}
	if p := r.Devices[1]; p.Path != "0000:01:00.0" || p.Vendor != "1002" || p.Driver != "amdgpu" {
		t.Errorf("pci device was altered: %+v", p)
	}
	// Nothing identifying left anywhere in the JSON.
	b, _ := json.Marshal(r)
	for _, bad := range []string{"my-secret-box", "12:34:56", "ABC123"} {
		if strings.Contains(string(b), bad) {
			t.Errorf("anonymized JSON still contains %q", bad)
		}
	}
}
