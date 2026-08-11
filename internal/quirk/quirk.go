package quirk

// Quirk represents a known hardware quirk with provenance
type Quirk struct {
	ID          string            `json:"id"`           // unique quirk ID
	DeviceIDs   []string          `json:"device_ids"`   // PCI/USB vendor:device IDs this applies to
	DeviceNames []string          `json:"device_names"` // human-readable device names
	Category    string            `json:"category"`     // gpu, usb, storage, audio, network, apple
	Module      string            `json:"module"`       // kernel module this quirk affects
	Params      map[string]string `json:"params"`       // module parameters to set
	Description string            `json:"description"`  // what the quirk is
	Reason      string            `json:"reason"`       // why this param is needed
	Citation    string            `json:"citation"`     // URL or reference to source
	KernelMin   string            `json:"kernel_min"`   // minimum kernel version (empty = all)
	KernelMax   string            `json:"kernel_max"`   // maximum kernel version (empty = all)
	Severity    string            `json:"severity"`    // "fatal", "warning", "info"
	Verified    bool              `json:"verified"`     // verified on real hardware
}

// QuirkDatabase is the embedded quirk database
type QuirkDatabase struct {
	Quirks []Quirk `json:"quirks"`
}

// Embedded quirks — each entry has provenance
var Database = QuirkDatabase{
	Quirks: []Quirk{
		// AMD FirePro D700 / Tahiti XT (Annie's GPUs)
		{
			ID:          "amdgpu-tahiti-xt-si-support",
			DeviceIDs:   []string{"1002:6799", "1002:6798", "1002:679a"},
			DeviceNames: []string{"AMD FirePro D700 (Tahiti XT)", "AMD Radeon HD 7970 (Tahiti XT)"},
			Category:    "gpu",
			Module:      "amdgpu",
			Params:      map[string]string{"si_support": "1"},
			Description: "Tahiti XT silicon requires si_support=1 to initialize the SMC (System Management Controller) properly",
			Reason:      "The amdgpu driver defaults to si_support=0 for Southern Islands (SI) GPUs. Tahiti XT silicon has a bug in the SMC initialization sequence that causes the driver to fail loading without this parameter. The Mac Pro 6,1 ships with Tahiti XT GPUs labeled as 'FirePro D300' or 'D700' but the silicon is full Tahiti XT.",
			Citation:    "https://gitlab.freedesktop.org/agd5f/linux/-/issues/32; Arch Wiki: amdgpu#Southern Islands; lore.kernel.org LKML thread 'amdgpu: SI support'",
			KernelMin:   "5.0",
			KernelMax:   "",
			Severity:    "fatal",
			Verified:    true,
		},
		{
			ID:          "amdgpu-tahiti-xt-dc",
			DeviceIDs:   []string{"1002:6799", "1002:6798", "1002:679a"},
			DeviceNames: []string{"AMD FirePro D700 (Tahiti XT)", "AMD Radeon HD 7970 (Tahiti XT)"},
			Category:    "gpu",
			Module:      "amdgpu",
			Params:      map[string]string{"dc": "1"},
			Description: "Enable Display Core (DC) support for Tahiti XT to get display output",
			Reason:      "The DC component of amdgpu is disabled by default for SI GPUs. Without dc=1, the display pipeline doesn't initialize and you get a black screen even though the driver loads. On the Mac Pro 6,1, this is needed to get actual display output through the mini-DisplayPort connectors.",
			Citation:    "Arch Wiki: amdgpu#Display Core; freedesktop.org Bugzilla #111652",
			KernelMin:   "5.0",
			KernelMax:   "",
			Severity:    "warning",
			Verified:    true,
		},
		// Fresco Logic FL1100 USB 3.0 host controller (Annie's USB 3.0)
		{
			ID:          "fl1100-usb3-firmware",
			DeviceIDs:   []string{"1b73:1100"},
			DeviceNames: []string{"Fresco Logic FL1100 USB 3.0 Host Controller"},
			Category:    "usb",
			Module:      "xhci_hcd",
			Params:      map[string]string{},
			Description: "FL1100 has a known firmware timing issue on Apple hardware that can cause intermittent USB 3.0 disconnects",
			Reason:      "The FL1100 USB 3.0 controller in the Mac Pro 6,1 has timing mismatches with Apple's USB-C connector wiring. Symptoms: USB 3.0 devices randomly disconnect, then reconnect at USB 2.0 speed. No module param fixes this — it's a hardware/firmware issue. Workaround: use a powered USB 3.0 hub.",
			Citation:    "Apple Support Communities: Mac Pro 6,1 USB 3.0 issues; Fresco Logic FL1100 errata sheet",
			KernelMin:   "",
			KernelMax:   "",
			Severity:    "info",
			Verified:    true,
		},
		// Apple SMC (System Management Controller) on Mac Pro 6,1
		{
			ID:          "apple-smc-macpro61",
			DeviceIDs:   []string{"applesmc"},
			DeviceNames: []string{"Apple SMC (Mac Pro 6,1)"},
			Category:    "apple",
			Module:      "applesmc",
			Params:      map[string]string{},
			Description: "Apple SMC on the Mac Pro 6,1 exposes temperature sensors, fan control, and LED control via the applesmc module",
			Reason:      "The applesmc module works out of the box on CachyOS for the Mac Pro 6,1. However, some sensor keys differ from older Mac Pros. The module auto-detects the correct key map. If fan control is needed, load the 'applesmc' module and write to /sys/devices/platform/applesmc.768/fan1_manual and /sys/devices/platform/applesmc.768/fan1_target.",
			Citation:    "kernel.org: drivers/hwmon/applesmc.c; Arch Wiki: Mac Pro 6,1",
			KernelMin:   "4.0",
			KernelMax:   "",
			Severity:    "info",
			Verified:    true,
		},
		// Apple gmux (graphics multiplexer) on Mac Pro 6,1
		{
			ID:          "apple-gmux-macpro61",
			DeviceIDs:   []string{"apple-gmux"},
			DeviceNames: []string{"Apple GMUX (Graphics Multiplexer)"},
			Category:    "apple",
			Module:      "apple-gmux",
			Params:      map[string]string{},
			Description: "GMUX on Mac Pro 6,1 switches between dual GPUs. On Linux, both GPUs are visible but gmux switching is limited.",
			Reason:      "The Mac Pro 6,1 has two FirePro GPUs connected via gmux. On Linux, both GPUs are typically active simultaneously (no dynamic switching like macOS). The apple-gmux module loads but GPU switching is not fully supported. For dual-display, both GPUs drive their own displays independently.",
			Citation:    "kernel.org: drivers/platform/x86/apple-gmux.c; Arch Wiki: MacBook Pro dual graphics",
			KernelMin:   "3.15",
			KernelMax:   "",
			Severity:    "info",
			Verified:    true,
		},
		// NVMe power management on certain controllers
		{
			ID:          "nvme-apst-disable",
			DeviceIDs:   []string{"nvme"},
			DeviceNames: []string{"Various NVMe SSDs"},
			Category:    "storage",
			Module:      "nvme",
			Params:      map[string]string{"nvme_core.default_ps_max_latency_us": "0"},
			Description: "Some NVMe SSDs have APST (Autonomous Power State Transition) bugs causing crashes. Disable APST as workaround.",
			Reason:      "Certain NVMe SSDs (notably early Samsung 950 Pro, some Crucial P1 firmware revisions) crash when APST transitions to deep sleep states. Disabling APST (setting default_ps_max_latency_us=0) prevents the crashes at the cost of higher idle power consumption.",
			Citation:    "kernel.org: drivers/nvme/host/core.c; LKML: 'NVMe APST crashes on Samsung 950 Pro'",
			KernelMin:   "4.11",
			KernelMax:   "",
			Severity:    "warning",
			Verified:    false,
		},
		// Intel HD Audio on certain motherboards
		{
			ID:          "snd-hda-intel-model-fixup",
			DeviceIDs:   []string{"8086:0c0c", "8086:8c20", "8086:a170"},
			DeviceNames: []string{"Intel HD Audio (various)"},
			Category:    "audio",
			Module:      "snd_hda_intel",
			Params:      map[string]string{"model": "auto"},
			Description: "Some Intel HD Audio controllers need explicit model parameter to detect correct codec configuration",
			Reason:      "On certain motherboards, the codec auto-detection fails and the wrong pin configuration is applied, resulting in non-working audio jacks. Setting model=auto forces a full re-probe which sometimes works better than the auto-detected configuration.",
			Citation:    "kernel.org: Documentation/sound/hd-audio/models.rst; Arch Wiki: Advanced Linux Sound Architecture",
			KernelMin:   "",
			KernelMax:   "",
			Severity:    "info",
			Verified:    false,
		},
	},
}

// FindByDevice returns quirks that match a given PCI vendor:device ID.
// Also matches non-PCI device IDs (e.g. "nvme", "applesmc") when they
// appear as device identifiers rather than module names.
func FindByDevice(vendor, deviceID string) []Quirk {
	id := vendor + ":" + deviceID
	var matches []Quirk
	for _, q := range Database.Quirks {
		for _, did := range q.DeviceIDs {
			// PCI vendor:device ID match
			if did == id {
				matches = append(matches, q)
				break
			}
			// Non-PCI device ID match (platform devices, module names)
			// These match when the query ID equals the quirk's device ID directly
			if did == deviceID && (did == "nvme" || did == "applesmc" || did == "apple-gmux") {
				matches = append(matches, q)
				break
			}
		}
	}
	return matches
}

// FindByModule returns quirks that match a given kernel module
func FindByModule(module string) []Quirk {
	var matches []Quirk
	for _, q := range Database.Quirks {
		if q.Module == module {
			matches = append(matches, q)
		}
	}
	return matches
}

// FindAll returns all known quirks
func FindAll() []Quirk {
	return Database.Quirks
}