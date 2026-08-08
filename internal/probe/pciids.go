package probe

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"sync"
)

// PCIDB represents the parsed pci.ids database
type PCIDB struct {
	Vendors map[string]string            // vendor_id -> vendor_name
	Devices map[string]map[string]string // vendor_id -> {device_id -> device_name}
	once    sync.Once
	loaded  bool
}

var db PCIDB

// LoadPCIDB lazily loads the pci.ids database
func LoadPCIDB() {
	db.once.Do(func() {
		db.Vendors = make(map[string]string)
		db.Devices = make(map[string]map[string]string)

		// Try standard locations
		paths := []string{
			"/usr/share/hwdata/pci.ids",
			"/usr/share/pci.ids",
			"/usr/share/misc/pci.ids",
		}

		var file *os.File
		var err error
		for _, path := range paths {
			file, err = os.Open(path)
			if err == nil {
				break
			}
		}

		if file == nil {
			// No pci.ids found — descriptions will remain empty
			return
		}
		defer file.Close()

		scanner := bufio.NewScanner(file)
		var currentVendor string

		for scanner.Scan() {
			line := scanner.Text()

			// Skip comments and blank lines
			if len(line) == 0 || line[0] == '#' {
				continue
			}

			// No tabs = vendor line
			if line[0] != '\t' {
				fields := strings.SplitN(line, "  ", 2)
				if len(fields) == 2 {
					vendorID := strings.ToLower(strings.TrimSpace(fields[0]))
					vendorName := strings.TrimSpace(fields[1])
					db.Vendors[vendorID] = vendorName
					currentVendor = vendorID
					if db.Devices[vendorID] == nil {
						db.Devices[vendorID] = make(map[string]string)
					}
				}
				continue
			}

			// One tab = device line (under current vendor)
			if currentVendor != "" && !strings.HasPrefix(line, "\t\t") {
				deviceLine := strings.TrimPrefix(line, "\t")
				fields := strings.SplitN(deviceLine, "  ", 2)
				if len(fields) == 2 {
					deviceID := strings.ToLower(strings.TrimSpace(fields[0]))
					deviceName := strings.TrimSpace(fields[1])
					db.Devices[currentVendor][deviceID] = deviceName
				}
			}
			// Two tabs = subsystem line — skip for now
		}

		db.loaded = true
	})
}

// LookupPCIVendor returns the vendor name for a PCI vendor ID
func LookupPCIVendor(vendorID string) string {
	LoadPCIDB()
	vendorID = strings.ToLower(strings.TrimPrefix(vendorID, "0x"))
	if name, ok := db.Vendors[vendorID]; ok {
		return name
	}
	return ""
}

// LookupPCIDevice returns the device description for a PCI vendor:device pair
func LookupPCIDevice(vendorID, deviceID string) string {
	LoadPCIDB()
	vendorID = strings.ToLower(strings.TrimPrefix(vendorID, "0x"))
	deviceID = strings.ToLower(strings.TrimPrefix(deviceID, "0x"))

	if devices, ok := db.Devices[vendorID]; ok {
		if name, ok := devices[deviceID]; ok {
			return name
		}
	}

	// Fallback: vendor name only
	vendorName := LookupPCIVendor(vendorID)
	if vendorName != "" {
		return fmt.Sprintf("%s device %s", vendorName, deviceID)
	}

	return ""
}

// init loads the PCI database on first use