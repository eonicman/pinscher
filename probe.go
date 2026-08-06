package probe

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Device represents a single hardware device discovered via sysfs
type Device struct {
	Bus         string `json:"bus"`          // pci, usb, platform, etc.
	Path        string `json:"path"`         // sysfs path
	Vendor      string `json:"vendor"`       // vendor ID (e.g. "1002" for AMD)
	DeviceID    string `json:"device_id"`    // device ID
	Subsystem   string `json:"subsystem"`    // subsystem vendor:device
	Class       string `json:"class"`        // device class (e.g. "030000" for VGA)
	Modalias    string `json:"modalias"`     // kernel modalias string
	Driver      string `json:"driver"`       // loaded driver (empty if none)
	Description string `json:"description"`  // human-readable description from pci.ids/usb.ids
	Revision    string `json:"revision"`     // hardware revision
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
	Total      int `json:"total"`
	WithDriver int `json:"with_driver"`
	NoDriver   int `json:"no_driver"`
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

		d.Vendor = readSysFile(filepath.Join(path, "vendor"))
		d.DeviceID = readSysFile(filepath.Join(path, "device"))
		d.Subsystem = readSysFile(filepath.Join(path, "subsystem_vendor")) + ":" + readSysFile(filepath.Join(path, "subsystem_device"))
		d.Class = stripHex(readSysFile(filepath.Join(path, "class")))
		d.Revision = readSysFile(filepath.Join(path, "revision"))
		d.Modalias = readSysFile(filepath.Join(path, "modalias"))

		// Check for loaded driver
		driverLink := filepath.Join(path, "driver")
		if driverPath, err := os.Readlink(driverLink); err == nil {
			d.Driver = filepath.Base(driverPath)
		}

		// Try to get description from PCI ID database
		d.Description = lookupPCIDescription(d.Vendor, d.DeviceID)

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

// stripHex removes 0x prefix from hex values
func stripHex(s string) string {
	return strings.TrimPrefix(s, "0x")
}

// readSysFile reads a sysfs/proc file and returns trimmed content
func readSysFile(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// lookupPCIDescription tries to look up a human-readable description from pci.ids
// This is a simple lookup — Phase 2 will embed the full pci.ids database
var pciIDCache map[string]string

func lookupPCIDescription(vendor, device string) string {
	if pciIDCache == nil {
		pciIDCache = make(map[string]string)
	}
	key := vendor + ":" + device
	if desc, ok := pciIDCache[key]; ok {
		return desc
	}

	// Try reading from /usr/share/hwdata/pci.ids
	// For now, return empty — Phase 2 will embed a full database
	return ""
}

// JSON returns the probe result as formatted JSON
func (r *ProbeResult) JSON() (string, error) {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}