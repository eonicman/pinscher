package probe

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Device represents a single hardware device discovered via sysfs
type Device struct {
	Bus          string            `json:"bus"`                    // pci, usb, platform, acpi, bluetooth, wifi, etc.
	Path         string            `json:"path"`                   // sysfs path
	Vendor       string            `json:"vendor"`                 // vendor ID (e.g. "1002" for AMD)
	DeviceID     string            `json:"device_id"`              // device ID
	Subsystem    string            `json:"subsystem"`              // subsystem vendor:device
	Class        string            `json:"class"`                  // device class (e.g. "030000" for VGA)
	ClassName    string            `json:"class_name"`             // human-readable class name (e.g. "VGA controller")
	Modalias     string            `json:"modalias"`               // kernel modalias string
	Driver       string            `json:"driver"`                 // loaded driver (empty if none)
	Description  string            `json:"description"`            // human-readable description from pci.ids/usb.ids
	Revision     string            `json:"revision"`               // hardware revision
	Capabilities map[string]string `json:"capabilities,omitempty"` // device capabilities/features
}

// ProbeResult is the full hardware report
type ProbeResult struct {
	Hostname string   `json:"hostname"`
	Kernel   string   `json:"kernel"`
	Devices  []Device `json:"devices"`
	Summary  Summary  `json:"summary"`
}

// Summary counts devices by bus and driver status
type Summary struct {
	Total      int            `json:"total"`
	WithDriver int            `json:"with_driver"`
	NoDriver   int            `json:"no_driver"`
	ByBus      map[string]int `json:"by_bus"`
}

// Probe walks sysfs and discovers all hardware devices
func Probe() (*ProbeResult, error) {
	hostname, _ := os.Hostname()
	kernel := readSysFile("/proc/sys/kernel/osrelease")

	result := &ProbeResult{
		Hostname: hostname,
		Kernel:   kernel,
		Summary:  Summary{ByBus: make(map[string]int)},
	}

	// Probe PCI devices
	pciDevices, err := probePCI()
	if err == nil {
		result.Devices = append(result.Devices, pciDevices...)
	}

	// Probe USB devices
	usbDevices, err := probeUSB()
	if err == nil {
		result.Devices = append(result.Devices, usbDevices...)
	}

	// Probe platform devices
	platformDevices, err := probePlatform()
	if err == nil {
		result.Devices = append(result.Devices, platformDevices...)
	}

	// Probe ACPI devices
	acpiDevices, err := probeACPI()
	if err == nil {
		result.Devices = append(result.Devices, acpiDevices...)
	}

	// Probe Bluetooth devices
	btDevices, err := probeBluetooth()
	if err == nil {
		result.Devices = append(result.Devices, btDevices...)
	}

	// Probe WiFi devices
	wifiDevices, err := probeWiFi()
	if err == nil {
		result.Devices = append(result.Devices, wifiDevices...)
	}

	// Sort devices by bus then path
	sort.Slice(result.Devices, func(i, j int) bool {
		if result.Devices[i].Bus != result.Devices[j].Bus {
			return result.Devices[i].Bus < result.Devices[j].Bus
		}
		return result.Devices[i].Path < result.Devices[j].Path
	})

	// Build summary
	for _, d := range result.Devices {
		result.Summary.Total++
		result.Summary.ByBus[d.Bus]++
		if d.Driver != "" {
			result.Summary.WithDriver++
		} else {
			result.Summary.NoDriver++
		}
	}

	return result, nil
}

// probePCI walks /sys/bus/pci/devices/
func probePCI() ([]Device, error) {
	devices, err := os.ReadDir("/sys/bus/pci/devices/")
	if err != nil {
		return nil, fmt.Errorf("cannot read PCI bus: %w", err)
	}

	var result []Device
	for _, entry := range devices {
		path := filepath.Join("/sys/bus/pci/devices/", entry.Name())

		d := Device{
			Bus:  "pci",
			Path: path,
		}

		d.Vendor = strings.TrimPrefix(readSysFile(filepath.Join(path, "vendor")), "0x")
		d.DeviceID = strings.TrimPrefix(readSysFile(filepath.Join(path, "device")), "0x")
		d.Subsystem = strings.TrimPrefix(readSysFile(filepath.Join(path, "subsystem_vendor")), "0x") + ":" + strings.TrimPrefix(readSysFile(filepath.Join(path, "subsystem_device")), "0x")
		d.Class = readSysFile(filepath.Join(path, "class"))
		d.Revision = readSysFile(filepath.Join(path, "revision"))
		d.Modalias = readSysFile(filepath.Join(path, "modalias"))

		// Check for loaded driver
		driverLink := filepath.Join(path, "driver")
		if driverPath, err := os.Readlink(driverLink); err == nil {
			d.Driver = filepath.Base(driverPath)
		}

		// Try to get description from PCI ID database
		d.Description = LookupPCIDevice(d.Vendor, d.DeviceID)

		// Resolve class name
		d.ClassName = classNameForPCI(d.Class)

		result = append(result, d)
	}

	return result, nil
}

// probeUSB walks /sys/bus/usb/devices/
func probeUSB() ([]Device, error) {
	devices, err := os.ReadDir("/sys/bus/usb/devices/")
	if err != nil {
		return nil, fmt.Errorf("cannot read USB bus: %w", err)
	}

	var result []Device
	// Only include actual USB devices (not USB host controllers which appear on PCI)
	for _, entry := range devices {
		name := entry.Name()
		// USB devices have names like "1-2", "2-1.3" — skip host controllers ("usb1", "usb2")
		matched, _ := regexp.MatchString(`^\d+-\d`, name)
		if !matched {
			continue
		}

		path := filepath.Join("/sys/bus/usb/devices/", name)

		d := Device{
			Bus:  "usb",
			Path: path,
		}

		d.Vendor = readSysFile(filepath.Join(path, "idVendor"))
		d.DeviceID = readSysFile(filepath.Join(path, "idProduct"))
		d.Revision = readSysFile(filepath.Join(path, "bcdDevice"))
		d.Modalias = readSysFile(filepath.Join(path, "modalias"))

		// USB class/subclass
		usbClass := readSysFile(filepath.Join(path, "bDeviceClass"))
		usbSub := readSysFile(filepath.Join(path, "bDeviceSubClass"))
		usbProto := readSysFile(filepath.Join(path, "bDeviceProtocol"))
		d.Class = fmt.Sprintf("%s%s%s", usbClass, usbSub, usbProto)

		// Product name from the device itself
		product := readSysFile(filepath.Join(path, "product"))
		manufacturer := readSysFile(filepath.Join(path, "manufacturer"))
		if product != "" {
			if manufacturer != "" {
				d.Description = manufacturer + " " + product
			} else {
				d.Description = product
			}
		}

		// Resolve USB class name
		d.ClassName = classNameForUSB(usbClass)

		// Check for loaded driver
		// USB devices have interfaces under the device directory
		driverLink := filepath.Join(path, "driver")
		if driverPath, err := os.Readlink(driverLink); err == nil {
			d.Driver = filepath.Base(driverPath)
		} else {
			// Check interface directories for drivers
			ifaces, _ := os.ReadDir(path)
			for _, iface := range ifaces {
				if !strings.Contains(iface.Name(), ":") {
					continue
				}
				ifaceDriverLink := filepath.Join(path, iface.Name(), "driver")
				if driverPath, err := os.Readlink(ifaceDriverLink); err == nil {
					if d.Driver == "" {
						d.Driver = filepath.Base(driverPath)
					} else {
						d.Driver += "," + filepath.Base(driverPath)
					}
				}
			}
		}

		result = append(result, d)
	}

	return result, nil
}

// pciClassName maps PCI class codes to human-readable names
var pciClassName = map[string]string{
	// Base classes (2-digit)
	"00": "Legacy Device",
	"01": "Storage Controller",
	"02": "Network Controller",
	"03": "Display Controller",
	"04": "Multimedia Controller",
	"05": "Memory Controller",
	"06": "Bridge Device",
	"07": "Communication Controller",
	"08": "Generic System Peripheral",
	"09": "Input Device Controller",
	"0a": "Docking Station",
	"0b": "Processor",
	"0c": "Serial Bus Controller",
	"0d": "Wireless Controller",
	"0e": "Intelligent IO Controller",
	"0f": "Satellite Communication Controller",
	"10": "Encryption Controller",
	"11": "Signal Processing Controller",
	"12": "Processing Accelerator",
	"13": "Non-Essential Instrumentation",
	"40": "Co-Processor",
	"ff": "Other",
	// Subclasses (4-digit)
	"0100": "SCSI Controller",
	"0101": "IDE Controller",
	"0102": "Floppy Controller",
	"0104": "RAID Controller",
	"0105": "ATA Controller",
	"0106": "SATA Controller",
	"0107": "SAS Controller",
	"0108": "NVMe Controller",
	"0200": "Ethernet Controller",
	"0201": "Token Ring Controller",
	"0280": "Network Controller (Wireless)",
	"0300": "VGA Compatible Controller",
	"0301": "XGA Controller",
	"0302": "3D Controller",
	"0400": "Multimedia Video Controller",
	"0401": "Multimedia Audio Controller",
	"0402": "Computer Telephony Device",
	"0403": "Audio Device",
	"0500": "RAM Controller",
	"0501": "Memory Controller",
	"0600": "Host Bridge",
	"0601": "ISA Bridge",
	"0604": "PCI-to-PCI Bridge",
	"0605": "PCMCIA Bridge",
	"0609": "Semi-transparent PCI-to-PCI Bridge",
	"0700": "Serial Controller",
	"0703": "Modem",
	"0800": "PIC",
	"0801": "DMA Controller",
	"0802": "Timer",
	"0803": "RTC",
	"0804": "PCI Hot-plug Controller",
	"0805": "SD Host Controller",
	"0806": "IOMMU",
	"0c03": "USB Controller",
	"0c05": "SMBus Controller",
	"0d01": "Wi-Fi Controller",
	"0d10": "WiMAX Controller",
	"1000": "Cryptography Controller",
	"1100": "Signal Processing Controller",
	"1200": "Processing Accelerator",
}

// classNameForPCI looks up a human-readable class name for a PCI class code
func classNameForPCI(class string) string {
	if len(class) >= 6 {
		// Try 6-char (bcXXscXXprXX) first, then 4-char (bcXXscXX)
		if name, ok := pciClassName[class[:6]]; ok {
			return name
		}
	}
	if len(class) >= 4 {
		if name, ok := pciClassName[class[:4]]; ok {
			return name
		}
	}
	if len(class) >= 2 {
		if name, ok := pciClassName[class[:2]]; ok {
			return name
		}
	}
	return ""
}

// usbClassName maps USB class codes to human-readable names
var usbClassName = map[string]string{
	"00": "Interface Defined",
	"01": "Audio",
	"02": "Communications and CDC Control",
	"03": "Human Interface Device (HID)",
	"05": "Physical",
	"06": "Image",
	"07": "Printer",
	"08": "Mass Storage",
	"09": "Hub",
	"0a": "CDC-Data",
	"0b": "Smart Card",
	"0d": "Content Security",
	"0e": "Video",
	"0f": "Personal Healthcare",
	"10": "Audio/Video Devices",
	"11": "Billboard Device",
	"12": "USB Type-C Bridge",
	"dc": "Diagnostic Device",
	"e0": "Wireless Controller",
	"ef": "Miscellaneous",
	"fe": "Application Specific",
	"ff": "Vendor Specific",
}

// classNameForUSB looks up a human-readable class name for a USB class code
func classNameForUSB(class string) string {
	if class != "" {
		if name, ok := usbClassName[strings.ToLower(class[:2])]; ok {
			return name
		}
	}
	return ""
}

// probePlatform walks /sys/bus/platform/devices/ for platform devices
func probePlatform() ([]Device, error) {
	devices, err := os.ReadDir("/sys/bus/platform/devices/")
	if err != nil {
		return nil, fmt.Errorf("cannot read platform bus: %w", err)
	}

	var result []Device
	for _, entry := range devices {
		path := filepath.Join("/sys/bus/platform/devices/", entry.Name())

		d := Device{
			Bus:  "platform",
			Path: path,
		}

		// Platform devices have a modalias that encodes their identity
		d.Modalias = readSysFile(filepath.Join(path, "modalias"))

		// Try to get a human-readable name from the modalias or device tree
		d.Description = readSysFile(filepath.Join(path, "name"))
		if d.Description == "" {
			// Use the directory entry name as fallback
			d.Description = entry.Name()
		}

		// Check for loaded driver
		driverLink := filepath.Join(path, "driver")
		if driverPath, err := os.Readlink(driverLink); err == nil {
			d.Driver = filepath.Base(driverPath)
		}

		// Platform devices may have OF (Open Firmware) compatible strings
		ofCompat := readSysFile(filepath.Join(path, "of_node/compatible"))
		if ofCompat != "" {
			d.Capabilities = map[string]string{"of_compatible": strings.ReplaceAll(ofCompat, "\x00", ", ")}
		}

		result = append(result, d)
	}

	return result, nil
}

// probeACPI walks /sys/bus/acpi/devices/ for ACPI devices
// Skips generic container objects (device:XX) that aren't real hardware
func probeACPI() ([]Device, error) {
	devices, err := os.ReadDir("/sys/bus/acpi/devices/")
	if err != nil {
		return nil, fmt.Errorf("cannot read ACPI bus: %w", err)
	}

	var result []Device
	for _, entry := range devices {
		name := entry.Name()

		// Skip generic container objects (device:XX) — they're ACPI scope
		// containers without real hardware behind them
		if strings.HasPrefix(name, "device:") {
			continue
		}

		path := filepath.Join("/sys/bus/acpi/devices/", name)

		d := Device{
			Bus:         "acpi",
			Path:        path,
			Description: name,
		}

		// ACPI HID (Hardware ID) — the standard identifier
		d.DeviceID = readSysFile(filepath.Join(path, "hid"))

		// Use HID for description if available
		if d.DeviceID != "" {
			d.Description = d.DeviceID
		}

		// ACPI CID (Compatible IDs) — additional compatible device IDs
		cid := readSysFile(filepath.Join(path, "cid"))
		if cid != "" {
			if d.Capabilities == nil {
				d.Capabilities = make(map[string]string)
			}
			d.Capabilities["acpi_cid"] = cid
		}

		// UID (Unique ID) — instance identifier
		uid := readSysFile(filepath.Join(path, "uid"))
		if uid != "" {
			if d.Capabilities == nil {
				d.Capabilities = make(map[string]string)
			}
			d.Capabilities["acpi_uid"] = uid
		}

		// Status — device status (present, enabled, functioning)
		status := readSysFile(filepath.Join(path, "status"))
		if status != "" {
			if d.Capabilities == nil {
				d.Capabilities = make(map[string]string)
			}
			d.Capabilities["acpi_status"] = status
		}

		// Check for loaded driver
		driverLink := filepath.Join(path, "driver")
		if driverPath, err := os.Readlink(driverLink); err == nil {
			d.Driver = filepath.Base(driverPath)
		}

		result = append(result, d)
	}

	return result, nil
}

// probeBluetooth walks /sys/class/bluetooth/ for Bluetooth HCI devices
func probeBluetooth() ([]Device, error) {
	entries, err := os.ReadDir("/sys/class/bluetooth/")
	if err != nil {
		return nil, nil // Not an error — Bluetooth may not be present
	}

	var result []Device
	for _, entry := range entries {
		hciName := entry.Name()
		path := filepath.Join("/sys/class/bluetooth/", hciName)

		d := Device{
			Bus:         "bluetooth",
			Path:        path,
			Description: "Bluetooth HCI " + hciName,
		}

		// Address
		d.DeviceID = readSysFile(filepath.Join(path, "address"))

		// Device type (LE, BR/EDR, etc.)
		devType := readSysFile(filepath.Join(path, "type"))
		caps := make(map[string]string)

		switch devType {
		case "0", "": // BR/EDR only
			caps["type"] = "BR/EDR"
		case "1":
			caps["type"] = "LE"
		case "2":
			caps["type"] = "BR/EDR/LE"
		default:
			caps["type"] = devType
		}

		// Name
		btName := readSysFile(filepath.Join(path, "name"))
		if btName != "" {
			d.Description = btName
		}

		// HCI version info
		hciVer := readSysFile(filepath.Join(path, "hci_version"))
		hciRev := readSysFile(filepath.Join(path, "hci_revision"))
		lmpVer := readSysFile(filepath.Join(path, "lmp_version"))
		lmpSubver := readSysFile(filepath.Join(path, "lmp_subversion"))

		if hciVer != "" {
			caps["hci_version"] = hciVer
		}
		if hciRev != "" {
			caps["hci_revision"] = hciRev
		}
		if lmpVer != "" {
			caps["lmp_version"] = lmpVer
		}
		if lmpSubver != "" {
			caps["lmp_subversion"] = lmpSubver
		}

		// Features — the LMP features bitfield tells us what the device can do
		features := readSysFile(filepath.Join(path, "features"))
		if features != "" {
			caps["lmp_features"] = features
		}

		// Manufacturer
		mfg := readSysFile(filepath.Join(path, "manufacturer"))
		if mfg != "" {
			caps["manufacturer"] = mfg
		}

		// Bus type (USB, UART, etc.)
		busType := readSysFile(filepath.Join(path, "bus"))
		if busType != "" {
			caps["bus"] = busType
		}

		// Resolve the device symlink to find vendor/product IDs from the parent bus
		// For USB Bluetooth, the device links to the USB interface (e.g. 1-1.1.3:1.0)
		// We need to use EvalSymlinks for proper path resolution
		deviceLink := filepath.Join(path, "device")
		if realPath, err := filepath.EvalSymlinks(deviceLink); err == nil {
			// For USB Bluetooth, the device points to the USB interface;
			// parent directory has idVendor/idProduct
			parentPath := filepath.Dir(realPath)
			vendorFile := filepath.Join(parentPath, "idVendor")
			if _, err := os.Stat(vendorFile); err == nil {
				d.Vendor = readSysFile(vendorFile)
				d.DeviceID = readSysFile(filepath.Join(parentPath, "idProduct"))
				// Also get product/manufacturer name
				product := readSysFile(filepath.Join(parentPath, "product"))
				manufacturer := readSysFile(filepath.Join(parentPath, "manufacturer"))
				if product != "" {
					if manufacturer != "" {
						d.Description = manufacturer + " " + product
					} else {
						d.Description = product
					}
				}
			}

			// Check for driver at the resolved device
			if d.Driver == "" {
				driverPath := filepath.Join(realPath, "driver")
				if driverTarget, err := os.Readlink(driverPath); err == nil {
					d.Driver = filepath.Base(driverTarget)
				}
			}
		}

		// Set vendor from manufacturer if still empty
		if d.Vendor == "" && mfg != "" {
			d.Vendor = mfg
		}

		d.Capabilities = caps
		result = append(result, d)
	}

	return result, nil
}

// probeWiFi walks /sys/class/ieee80211/ for WiFi PHY devices
func probeWiFi() ([]Device, error) {
	entries, err := os.ReadDir("/sys/class/ieee80211/")
	if err != nil {
		return nil, nil // Not an error — WiFi may not be present
	}

	var result []Device
	for _, entry := range entries {
		phyName := entry.Name() // e.g. "phy0"
		phyPath := filepath.Join("/sys/class/ieee80211/", phyName)

		caps := make(map[string]string)
		d := Device{
			Bus:         "wifi",
			Path:        phyPath,
			Description: "WiFi " + phyName,
		}

		// MAC address
		macAddr := readSysFile(filepath.Join(phyPath, "address"))
		d.DeviceID = macAddr

		// Check for driver — the PHY device links back to the parent device
		// Try device/link/ to find the parent bus device
		driverLink := filepath.Join(phyPath, "device/driver")
		if driverPath, err := os.Readlink(driverLink); err == nil {
			d.Driver = filepath.Base(driverPath)
		}

		// Try to get PCI address and vendor/device from linked device
		pciDeviceLink := filepath.Join(phyPath, "device")
		if pciDevicePath, err := os.Readlink(pciDeviceLink); err == nil {
			// Extract PCI address from link (e.g. ../../../devices/pci0000:00/0000:00:1c.0/...)
			// Walk up to find the PCI device
			resolved := filepath.Clean(pciDevicePath)
			parts := strings.Split(resolved, "/")
			for _, part := range parts {
				if strings.Contains(part, ":") && len(strings.Split(part, ":")) == 3 {
					d.Vendor = part // Store PCI address for cross-reference

					// Read vendor/device IDs from the PCI device
					pciPath := filepath.Join("/sys/bus/pci/devices/", part)
					if vendor := readSysFile(filepath.Join(pciPath, "vendor")); vendor != "" {
						d.Vendor = strings.TrimPrefix(vendor, "0x")
					}
					if devid := readSysFile(filepath.Join(pciPath, "device")); devid != "" {
						if d.DeviceID != "" {
							caps["mac_address"] = d.DeviceID // Save MAC to capabilities
						}
						d.DeviceID = strings.TrimPrefix(devid, "0x")
					}
					if desc := LookupPCIDevice(d.Vendor, d.DeviceID); desc != "" {
						d.Description = desc
					}
					break
				}
			}
		}

		// Hardware capabilities bitmask
		hwCaps := readSysFile(filepath.Join(phyPath, "hardware"))
		if hwCaps != "" {
			caps["hardware_caps"] = hwCaps
		}

		// Supported interface modes
		ifModes, err := os.ReadFile(filepath.Join(phyPath, "interface_modes"))
		if err == nil {
			caps["interface_modes"] = strings.TrimSpace(string(ifModes))
		}

		// Supported cipher suites
		ciphers, err := os.ReadFile(filepath.Join(phyPath, "ciphers"))
		if err == nil {
			caps["ciphers"] = strings.TrimSpace(string(ciphers))
		}

		// HT/VHT capable flags
		htCap := readSysFile(filepath.Join(phyPath, "ht_capable"))
		vhtCap := readSysFile(filepath.Join(phyPath, "vht_capable"))
		if htCap != "" {
			caps["ht_capable"] = htCap
		}
		if vhtCap != "" {
			caps["vht_capable"] = vhtCap
		}

		d.Capabilities = caps
		result = append(result, d)
	}

	// Also try to get detailed WiFi capabilities via iw
	wifiDetails := probeWiFiDetails()
	for i := range result {
		macAddr := result[i].DeviceID
		if orig := result[i].Capabilities["mac_address"]; orig != "" {
			macAddr = orig
		}
		for k, v := range wifiDetails[macAddr] {
			if result[i].Capabilities == nil {
				result[i].Capabilities = make(map[string]string)
			}
			result[i].Capabilities[k] = v
		}
	}

	return result, nil
}

// probeWiFiDetails runs `iw phy# info` to get detailed WiFi capabilities
func probeWiFiDetails() map[string]map[string]string {
	result := make(map[string]map[string]string)

	// Find WiFi PHYs
	entries, err := os.ReadDir("/sys/class/ieee80211/")
	if err != nil {
		return result
	}

	for _, entry := range entries {
		phyName := entry.Name() // e.g. "phy0"
		phyNum := strings.TrimPrefix(phyName, "phy")

		cmd := exec.Command("iw", "phy"+phyNum, "info")
		output, err := cmd.Output()
		if err != nil {
			continue // iw may not be installed or PHY may be down
		}

		details := make(map[string]string)
		lines := strings.Split(string(output), "\n")

		var currentSection string
		for _, line := range lines {
			line = strings.TrimSpace(line)

			// Parse sections
			if strings.HasPrefix(line, "Frequencies:") {
				currentSection = "frequencies"
			} else if strings.HasPrefix(line, "Bitrates:") {
				currentSection = "bitrates"
			} else if strings.HasPrefix(line, "Supported interface modes") {
				currentSection = "iface_modes"
			} else if strings.HasPrefix(line, "Supported commands") {
				currentSection = "commands"
			} else if strings.HasPrefix(line, "Supported cipher") {
				currentSection = "ciphers"
			} else if strings.HasPrefix(line, "HT Capabilities") || strings.HasPrefix(line, "VHT Capabilities") {
				currentSection = "ht_vht"
			}

			// Extract key info
			if strings.HasPrefix(line, "\t") || strings.HasPrefix(line, "    ") {
				switch currentSection {
				case "frequencies":
					if strings.Contains(line, "MHz") && !strings.Contains(line, "disabled") {
						// Track bands
						freqStr := strings.TrimSpace(line)
						if strings.Contains(freqStr, "5") && strings.Contains(freqStr, "GHz") {
							details["band_5ghz"] = "supported"
						} else if strings.Contains(freqStr, "6") && strings.Contains(freqStr, "GHz") {
							details["band_6ghz"] = "supported"
						} else if strings.Contains(freqStr, "2") && strings.Contains(freqStr, "GHz") {
							details["band_2ghz"] = "supported"
						}
					}
				case "iface_modes":
					mode := strings.TrimSpace(strings.TrimPrefix(line, "\t"))
					mode = strings.TrimSpace(strings.TrimPrefix(mode, "* "))
					if mode != "" && !strings.HasPrefix(mode, "Frequencies") {
						if existing, ok := details["supported_modes"]; ok {
							details["supported_modes"] = existing + ", " + mode
						} else {
							details["supported_modes"] = mode
						}
					}
				}
			}
		}

		// Store by MAC address for cross-referencing
		macAddr := readSysFile(filepath.Join("/sys/class/ieee80211/", phyName, "address"))
		result[macAddr] = details
	}

	return result
}

// readSysFile reads a sysfs/proc file and returns trimmed content
func readSysFile(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// JSON returns the probe result as formatted JSON
func (r *ProbeResult) JSON() (string, error) {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}
