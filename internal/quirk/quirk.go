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
		// AMD FirePro D700 / Tahiti XT (Mac Pro 6,1 GPUs)
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
		// Fresco Logic FL1100 USB 3.0 host controller (Mac Pro 6,1 USB 3.0)
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

		// ===== ThinkPad quirks =====

		// ThinkPad X1 Carbon / T-series trackpoint scroll
		{
			ID:          "thinkpad-trackpoint-scroll",
			DeviceIDs:   []string{"04b3:3018", "04b3:3019", "17aa:5050"},
			DeviceNames: []string{"Lenovo ThinkPad TrackPoint", "Lenovo ThinkPad TrackPoint II"},
			Category:    "input",
			Module:      "psmouse",
			Params:      map[string]string{},
			Description: "ThinkPad TrackPoint middle-button scroll requires libinput configuration, not a kernel module param",
			Reason:      "Middle-button scroll on ThinkPad TrackPoints works via libinput's middle-button scrolling feature, not via psmouse module parameters. If scroll doesn't work, check Xorg/Wayland configuration: either set libinput middleemu enabled or use xinput set-prop. The psmouse module itself doesn't need special params on modern kernels.",
			Citation:    "Arch Wiki: TrackPoint; freedesktop.org: libinput middle-button scrolling",
			KernelMin:   "4.0",
			KernelMax:   "",
			Severity:    "info",
			Verified:    false,
		},
		// ThinkPad T480s / T490 Thunderbolt power management
		{
			ID:          "thinkpad-t480s-thunderbolt-power",
			DeviceIDs:   []string{"8086:15d2", "8086:15e7"},
			DeviceNames: []string{"Intel Thunderbolt 3 (T480s/T490)", "Intel Titan Ridge Thunderbolt 3"},
			Category:    "thunderbolt",
			Module:      "thunderbolt",
			Params:      map[string]string{},
			Description: "ThinkPad T480s/T490 Thunderbolt 3 firmware may need update for reliable hotplug. Also check IOMMU settings.",
			Reason:      "Early T480s/T490 Thunderbolt firmware had hotplug reliability issues. Update firmware via Lenovo's Thunderbolt firmware update tool (fwupd may have it). On Linux, ensure kernel IOMMU is enabled (intel_iommu=on) for DMA protection with Thunderbolt. Some T480s units need the Thunderbolt NVM firmware update from Lenovo.",
			Citation:    "Arch Wiki: Thunderbolt; Lenovo T480s Thunderbolt firmware update; fwupd.org",
			KernelMin:   "4.13",
			KernelMax:   "",
			Severity:    "warning",
			Verified:    false,
		},

		// ===== Framework Laptop quirks =====

		// Framework Laptop 13 (11th/12th gen) suspend/resume
		{
			ID:          "framework-13-s3-sleep",
			DeviceIDs:   []string{"8086:461e", "8086:a70e", "8086:7a70"},
			DeviceNames: []string{"Framework Laptop 13 (11th Gen)", "Framework Laptop 13 (12th Gen)", "Framework Laptop 13 (13th Gen)"},
			Category:    "power",
			Module:      "",
			Params:      map[string]string{},
			Description: "Framework Laptop 13 needs mem_sleep_default=s3 for proper S3 suspend. Modern standby (s0ix) has issues on some expansions.",
			Reason:      "Framework Laptops default to s0ix (modern standby) which can cause excessive battery drain and unreliable resume with certain expansion cards. Setting mem_sleep_default=s3 in kernel command line gives reliable deep sleep. Framework officially recommends this for 11th/12th gen; 13th gen has improved s0ix but S3 is still more reliable.",
			Citation:    "https://frame.work/linux; Arch Wiki: Framework laptop; community.frame.work Linux guide",
			KernelMin:   "5.4",
			KernelMax:   "",
			Severity:    "warning",
			Verified:    false,
		},
		// Framework Laptop audio codec fixup
		{
			ID:          "framework-13-audio-fixup",
			DeviceIDs:   []string{"1043:1a16", "1025:1535"},
			DeviceNames: []string{"Framework Laptop 13 Audio Codec (Realtek ALC295/ALC285)"},
			Category:    "audio",
			Module:      "snd_hda_intel",
			Params:      map[string]string{"model": "alc295-framework"},
			Description: "Framework Laptop 13 audio may need snd_hda_intel model=alc295-framework for correct headphone jack and speaker routing",
			Reason:      "The Realtek ALC295/ALC285 codec on Framework Laptops sometimes misconfigures pin routing, causing headphones to not be detected or speakers to not switch when headphones are plugged in. The alc295-framework model quirk fixes pin routing. Framework's official guide includes this fix.",
			Citation:    "https://frame.work/linux; Arch Wiki: Framework laptop audio",
			KernelMin:   "5.10",
			KernelMax:   "",
			Severity:    "info",
			Verified:    false,
		},

		// ===== Dell quirks =====

		// Dell XPS 13/15 webcam disable on suspend
		{
			ID:          "dell-xps-webcam-suspend",
			DeviceIDs:   []string{"0bda:5682", "1bcf:2c99"},
			DeviceNames: []string{"Dell XPS Webcam (Realtek/Sunplus)", "Dell Integrated Webcam HD"},
			Category:    "usb",
			Module:      "uvcvideo",
			Params:      map[string]string{"powersave": "0"},
			Description: "Dell XPS webcams can fail to resume from suspend. Disable uvcvideo powersave to fix.",
			Reason:      "Some Dell XPS 13/15 models have webcams (Realtek 5682 or Sunplus 1bcf:2c99) that fail to resume from suspend due to USB power management. The uvcvideo powersave=0 parameter prevents the kernel from suspending the webcam, which avoids the resume failure at the cost of slightly higher power consumption.",
			Citation:    "Dell Linux community; Arch Wiki: Dell XPS; bugzilla.kernel.org #205211",
			KernelMin:   "4.15",
			KernelMax:   "",
			Severity:    "warning",
			Verified:    false,
		},
		// Dell XPS 13 9360/9370 Thunderbolt
		{
			ID:          "dell-xps-thunderbolt",
			DeviceIDs:   []string{"8086:15d2"},
			DeviceNames: []string{"Intel Thunderbolt 3 (Dell XPS 13 9360/9370)"},
			Category:    "thunderbolt",
			Module:      "thunderbolt",
			Params:      map[string]string{},
			Description: "Dell XPS 13 9360/9370 Thunderbolt 3 needs firmware update and DMA security for reliable operation",
			Reason:      "The Alpine Ridge Thunderbolt 3 controller on Dell XPS 13 9360/9370 has known firmware issues causing hotplug failures and DMA errors. Update Thunderbolt firmware via fwupd or Dell's update tool. Enable IOMMU (intel_iommu=on) for DMA protection. Some units need the Thunderbolt NVM firmware update from Dell.",
			Citation:    "Dell Thunderbolt firmware guide; Arch Wiki: Dell XPS 13; fwupd.org",
			KernelMin:   "4.13",
			KernelMax:   "",
			Severity:    "warning",
			Verified:    false,
		},

		// ===== Raspberry Pi quirks =====

		// Raspberry Pi 4/5 USB-C power
		{
			ID:          "rpi4-usb-c-power",
			DeviceIDs:   []string{"1d6b:0002"},
			DeviceNames: []string{"Raspberry Pi 4/5 USB-C Power Delivery"},
			Category:    "power",
			Module:      "",
			Params:      map[string]string{},
			Description: "Raspberry Pi 4 USB-C power input doesn't work with some USB-C chargers due to non-compliant CC resistor",
			Reason:      "The original Raspberry Pi 4 (pre-1.4 revision) has a USB-C port that uses a single 5.1kΩ pull-down resistor instead of the correct two 5.1kΩ resistors on each CC line. This means many USB-C chargers (especially those with e-marked cables) won't provide power. Fixed in Pi 4 revision 1.4+. Workaround: use a USB-C to Micro-USB adapter, a USB-A charger, or a Pi 4 v1.4+.",
			Citation:    "Raspberry Pi Foundation: USB-C power issue; Tyler Ward hardware analysis; Arch Wiki: Raspberry Pi",
			KernelMin:   "",
			KernelMax:   "",
			Severity:    "info",
			Verified:    false,
		},

		// ===== NVIDIA quirks =====

		// NVIDIA proprietary driver power management
		{
			ID:          "nvidia-dynamic-power-management",
			DeviceIDs:   []string{"10de:1b80", "10de:1c03", "10de:1f91", "10de:2191", "10de:2560"},
			DeviceNames: []string{"NVIDIA GTX 1080 (GP104)", "NVIDIA GTX 1060 6GB (GP106)", "NVIDIA TU117M", "NVIDIA TU106 (RTX 2070)", "NVIDIA GA106 (RTX 3060)"},
			Category:    "gpu",
			Module:      "nvidia",
			Params:      map[string]string{"NVreg_DynamicPowerManagement": "0x02"},
			Description: "Enable fine-grained dynamic power management for NVIDIA GPUs to reduce idle power consumption",
			Reason:      "The NVIDIA proprietary driver supports Dynamic Power Management (DPM) with three modes: 0x00=off, 0x01=coarse (default), 0x02=fine. Fine-grained DPM allows the GPU to enter lower power states (RC6) during idle periods, significantly reducing power draw and heat. Essential for laptops and always-on desktops. Must be combined with DynamicPowerMangement in the Xorg config for full effect.",
			Citation:    "NVIDIA driver README: DynamicPowerManagement; Arch Wiki: NVIDIA#Power management",
			KernelMin:   "4.10",
			KernelMax:   "",
			Severity:    "info",
			Verified:    false,
		},

		// ===== AMD GPU quirks (beyond Mac Pro) =====

		// AMD GPU reset method
		{
			ID:          "amdgpu-reset-method",
			DeviceIDs:   []string{"1002:7480", "1002:744c", "1002:73a0", "1002:73e0", "1002:7483"},
			DeviceNames: []string{"AMD Radeon RX 7900 XTX (Navi 31)", "AMD Radeon RX 6800 XT (Navi 21)", "AMD Radeon RX 6700 XT (Navi 22)", "AMD Radeon RX 6600 XT (Navi 23)", "AMD Radeon RX 7600 (Navi 33)"},
			Category:    "gpu",
			Module:      "amdgpu",
			Params:      map[string]string{"reset_method": "3"},
			Description: "Modern AMD GPUs (RDNA2/RDNA3) may need reset_method=3 (mode3) for reliable GPU reset during VR/suspend",
			Reason:      "The amdgpu reset_method parameter controls how the GPU is reset when it hangs or during suspend/resume. Mode 3 (mode3) uses the GPU's internal reset logic, which is more reliable on RDNA2/RDNA3 GPUs than the default mode 1 (mode1) which can cause black screens or hangs on resume. This is especially important for VR headsets and suspend/resume cycles.",
			Citation:    "kernel.org: drivers/gpu/drm/amd/amdgpu/amdgpu_reset.c; freedesktop.org bug #1186; Arch Wiki: amdgpu#Reset method",
			KernelMin:   "5.15",
			KernelMax:   "",
			Severity:    "info",
			Verified:    false,
		},

		// ===== Audio quirks =====

		// Realtek ALC892/ALC897 headphone detection
		{
			ID:          "realtek-alc892-headphone",
			DeviceIDs:   []string{"10ec:0892", "10ec:0897"},
			DeviceNames: []string{"Realtek ALC892", "Realtek ALC897"},
			Category:    "audio",
			Module:      "snd_hda_intel",
			Params:      map[string]string{"model": "auto"},
			Description: "Realtek ALC892/ALC897 codecs may need model=auto for correct headphone jack detection",
			Reason:      "The ALC892 and ALC897 are common desktop audio codecs used on many motherboards. The default pin configuration detection sometimes fails to detect headphone insertion, causing audio to continue playing through speakers when headphones are plugged in. Setting model=auto forces a full codec re-probe.",
			Citation:    "kernel.org: Documentation/sound/hd-audio/models.rst; Arch Wiki: ALSA",
			KernelMin:   "",
			KernelMax:   "",
			Severity:    "info",
			Verified:    false,
		},

		// ===== Network quirks =====

		// Realtek RTL8125 2.5G Ethernet
		{
			ID:          "realtek-rtl8125-aspm",
			DeviceIDs:   []string{"10ec:8125"},
			DeviceNames: []string{"Realtek RTL8125 2.5GbE"},
			Category:    "network",
			Module:      "r8169",
			Params:      map[string]string{},
			Description: "Realtek RTL8125 2.5GbE may have ASPM (Active State Power Management) issues causing link drops. Disable ASPM on the PCIe slot as workaround.",
			Reason:      "The Realtek RTL8125 2.5GbE controller (used on many newer motherboards) has known ASPM issues where the link randomly drops to 100Mbps or disconnects entirely. This is a firmware/driver interaction bug. Workaround: add pcie_aspm=off to kernel command line or disable ASPM specifically for this device. The r8169 module itself doesn't have a parameter for this.",
			Citation:    "bugzilla.kernel.org #207553; Arch Wiki: Realtek RTL8125; Reddit: r/linuxhardware RTL8125 issues",
			KernelMin:   "5.10",
			KernelMax:   "",
			Severity:    "warning",
			Verified:    false,
		},
		// Intel AX200/AX210 WiFi
		{
			ID:          "intel-ax210-power-save",
			DeviceIDs:   []string{"8086:2725", "8086:2726", "8086:51f0", "8086:51f1"},
			DeviceNames: []string{"Intel AX200 WiFi 6", "Intel AX201 WiFi 6", "Intel AX210 WiFi 6E", "Intel AX211 WiFi 6E"},
			Category:    "network",
			Module:      "iwlwifi",
			Params:      map[string]string{"power_save": "0"},
			Description: "Intel AX200/AX210 WiFi cards may have stability issues with power saving enabled. Disable iwlwifi power_save for reliable connectivity.",
			Reason:      "The iwlwifi driver's power_save feature can cause intermittent disconnects, slow DNS resolution, and reduced throughput on Intel AX200/AX210/AX211 WiFi cards. This is a known issue. Setting power_save=0 disables power management on the WiFi card, increasing power consumption slightly but improving stability. Some users also need to disable 802.11ax (disable_11ax=1) for AX200 on older kernels.",
			Citation:    "Arch Wiki: Intel wireless; bugzilla.kernel.org #203709; Intel community forums",
			KernelMin:   "5.4",
			KernelMax:   "",
			Severity:    "warning",
			Verified:    false,
		},

		// ===== USB quirks =====

		// ASMedia ASM1142 USB 3.1 controller
		{
			ID:          "asm1142-usb31-hotplug",
			DeviceIDs:   []string{"1b21:1242"},
			DeviceNames: []string{"ASMedia ASM1142 USB 3.1 Host Controller"},
			Category:    "usb",
			Module:      "xhci_hcd",
			Params:      map[string]string{},
			Description: "ASMedia ASM1142 USB 3.1 controller may have hotplug issues. Check for firmware update.",
			Reason:      "The ASM1142 USB 3.1 (10Gbps) controller has known hotplug reliability issues, particularly with USB-C docks. The root cause is firmware timing. Updating the ASM1142 firmware via the manufacturer's update tool (or motherboard BIOS update) typically resolves this. No kernel module parameter fixes it.",
			Citation:    "ASMedia ASM1142 errata; Arch Wiki: USB; superuser.com ASM1142 threads",
			KernelMin:   "",
			KernelMax:   "",
			Severity:    "info",
			Verified:    false,
		},

		// ===== Storage quirks =====

		// Samsung NVMe 970 EVO Plus firmware
		{
			ID:          "samsung-970evo-plus-firmware",
			DeviceIDs:   []string{"144d:a808", "144d:a809"},
			DeviceNames: []string{"Samsung SSD 970 EVO Plus", "Samsung SSD 980 PRO"},
			Category:    "storage",
			Module:      "nvme",
			Params:      map[string]string{},
			Description: "Samsung 970 EVO Plus and 980 PRO have firmware bugs that cause NVMe command timeouts. Update firmware to latest version.",
			Reason:      "Multiple Samsung NVMe SSD firmware versions (particularly on the 970 EVO Plus 2TB and 980 PRO) have known bugs causing random command timeouts, I/O errors, and in severe cases data corruption. Samsung has released firmware updates that fix these issues. Update via Samsung Magician (Windows) or samsung-nvm-update (bootable ISO). No kernel parameter reliably works around all variants.",
			Citation:    "Samsung firmware updates; bbs.archlinux.org Samsung 970 EVO Plus issues; kernel.org: drivers/nvme/host/pci.c quirks",
			KernelMin:   "",
			KernelMax:   "",
			Severity:    "fatal",
			Verified:    false,
		},

		// ===== Apple hardware (beyond Mac Pro) =====

		// MacBook Pro 2016-2018 keyboard backlight / Touch Bar
		{
			ID:          "macbookpro-touchbar-applespi",
			DeviceIDs:   []string{"05ac:8106", "05ac:0265"},
			DeviceNames: []string{"Apple Touch Bar (MacBook Pro)", "Apple SPI Keyboard/Trackpad"},
			Category:    "input",
			Module:      "applespi",
			Params:      map[string]string{},
			Description: "MacBook Pro 2016-2018 Touch Bar and keyboard need applespi module and proper SPI configuration",
			Reason:      "The MacBook Pro 2016-2018 models use SPI-based keyboard and trackpad (not USB/PS2). The applespi module is required for keyboard/trackpad function. On older kernels (< 5.4), it may need intel_lpss (Intel Low Power Subsystem) loaded first. The Touch Bar uses a separate USB interface. Both require proper ACPI configuration. On T2-based Macs (2018+), the touchbar and audio need apple-ib-tb module.",
			Citation:    "Arch Wiki: MacBook Pro; github.com/roadrunner2/macbook12-spi-driver; github.com/Aunty1/apple-ib-tb",
			KernelMin:   "5.4",
			KernelMax:   "",
			Severity:    "warning",
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