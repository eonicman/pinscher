package diagnose

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/eonicman/pinscher/internal/probe"
	"github.com/eonicman/pinscher/internal/quirk"
)

// DiagnoseResult is the output of pinscher diagnose
type DiagnoseResult struct {
	Device      probe.Device    `json:"device"`
	Driver      string          `json:"driver"` // loaded driver name (empty if none)
	Status      string          `json:"status"` // "ok", "no_driver", "driver_loaded_but_broken", "quirk_needed"
	Issues      []DiagnoseIssue `json:"issues"`
	Quirks      []quirk.Quirk   `json:"quirks"`      // applicable quirks
	Suggestions []string        `json:"suggestions"` // human-readable fix suggestions
}

// DiagnoseIssue represents a specific problem found during diagnosis
type DiagnoseIssue struct {
	Severity string `json:"severity"` // "fatal", "warning", "info"
	Class    string `json:"class"`    // "no_module", "blacklisted", "firmware_missing", "param_wrong", "hardware_error", "quirk"
	Module   string `json:"module"`   // related module
	Message  string `json:"message"`  // what's wrong
	Fix      string `json:"fix"`      // how to fix it
	Source   string `json:"source"`   // "sysfs", "dmesg", "quirk_db", "modprobe"
}

// Diagnose performs a deep diagnosis of a single device
func Diagnose(devicePath string) (*DiagnoseResult, error) {
	// 1. Find the device in sysfs
	probeResult, err := probe.Probe()
	if err != nil {
		return nil, fmt.Errorf("probe failed: %w", err)
	}
	dev, err := probe.FindDevice(probeResult, devicePath)
	if err != nil {
		return nil, err
	}

	result := &DiagnoseResult{
		Device: *dev,
		Driver: dev.Driver,
	}

	// 2. Determine status
	if dev.Driver == "" {
		result.Status = "no_driver"
		diagnoseNoDriver(dev, result)
	} else {
		result.Status = "ok"
		diagnoseDriverLoaded(dev, result)
	}

	// 3. Check quirk database
	checkQuirks(dev, result)

	// 4. Scan dmesg for relevant errors
	dmesgIssues := scanDmesg(dev)
	result.Issues = append(result.Issues, dmesgIssues...)

	// 5. Generate suggestions
	result.Suggestions = generateSuggestions(result)

	// 6. Update status based on issues
	for _, issue := range result.Issues {
		if issue.Severity == "fatal" {
			if result.Status == "ok" {
				result.Status = "driver_loaded_but_broken"
			}
		}
	}

	return result, nil
}

// findDevice locates a device in sysfs by path, PCI address, or partial match
// findDevice used to live here as its own copy of the matching logic; moved
// to probe.FindDevice (2026-10-17, Ghost) so diagnose/quirk-submit/lint-boot
// --assert share one implementation instead of three drifting copies.

// diagnoseNoDriver investigates why a device has no driver loaded
func diagnoseNoDriver(dev *probe.Device, result *DiagnoseResult) {
	module := resolveModalias(dev.Modalias)
	if module == "" {
		result.Issues = append(result.Issues, DiagnoseIssue{
			Severity: "fatal",
			Class:    "no_module",
			Module:   "",
			Message:  fmt.Sprintf("No kernel module found for modalias '%s' — no driver exists in the kernel module tree", dev.Modalias),
			Fix:      "Check if the device is supported by an out-of-tree module or DKMS package. Search for the vendor:device ID online.",
			Source:   "modprobe",
		})
		return
	}

	// Module exists — check why it's not loaded
	if isBlacklisted(module) {
		result.Issues = append(result.Issues, DiagnoseIssue{
			Severity: "fatal",
			Class:    "blacklisted",
			Module:   module,
			Message:  fmt.Sprintf("Module '%s' is blacklisted — it won't load automatically or manually", module),
			Fix:      findBlacklistSource(module),
			Source:   "modprobe",
		})
		return
	}

	// Check if firmware is missing
	if firmwareMissing := checkFirmware(dev, module); firmwareMissing != "" {
		result.Issues = append(result.Issues, DiagnoseIssue{
			Severity: "fatal",
			Class:    "firmware_missing",
			Module:   module,
			Message:  fmt.Sprintf("Module '%s' requires firmware '%s' which is not in /lib/firmware/", module, firmwareMissing),
			Fix:      fmt.Sprintf("Install the linux-firmware package or download the firmware file to /lib/firmware/%s", firmwareMissing),
			Source:   "sysfs",
		})
		return
	}

	// Module exists, not blacklisted, not firmware — just not loaded
	result.Issues = append(result.Issues, DiagnoseIssue{
		Severity: "warning",
		Class:    "no_module",
		Module:   module,
		Message:  fmt.Sprintf("Module '%s' exists but is not loaded — device may work after 'sudo modprobe %s'", module, module),
		Fix:      fmt.Sprintf("sudo modprobe %s", module),
		Source:   "sysfs",
	})
}

// diagnoseDriverLoaded checks if a loaded driver is actually working
func diagnoseDriverLoaded(dev *probe.Device, result *DiagnoseResult) {
	// Check sysfs for error indicators
	errorPath := filepath.Join(dev.Path, "error")
	if data, err := os.ReadFile(errorPath); err == nil {
		errStr := strings.TrimSpace(string(data))
		if errStr != "" && errStr != "0" {
			result.Issues = append(result.Issues, DiagnoseIssue{
				Severity: "warning",
				Class:    "hardware_error",
				Module:   dev.Driver,
				Message:  fmt.Sprintf("Device reports error code: %s", errStr),
				Fix:      "Check dmesg for details. Consider reloading the driver: sudo rmmod <module> && sudo modprobe <module>",
				Source:   "sysfs",
			})
		}
	}

	// Check for driver-specific error files
	for _, errFile := range []string{"aer_error", "devlink/err", "uevent"} {
		path := filepath.Join(dev.Path, errFile)
		if _, err := os.Stat(path); err == nil {
			// File exists — for now just note it. Phase 3 will parse these.
		}
	}
}

// checkQuirks looks up the device in the quirk database
func checkQuirks(dev *probe.Device, result *DiagnoseResult) {
	quirks := quirk.FindByDevice(dev.Vendor, dev.DeviceID)
	if len(quirks) == 0 {
		// Try by module
		if dev.Driver != "" {
			quirks = quirk.FindByModule(dev.Driver)
		}
	}

	for _, q := range quirks {
		result.Quirks = append(result.Quirks, q)

		// Check if the quirk params are already applied
		for param, expectedValue := range q.Params {
			currentValue := readModuleParam(dev.Driver, param)
			if currentValue == "" {
				// Param not set — check if it's needed
				if q.Severity == "fatal" || q.Severity == "warning" {
					result.Issues = append(result.Issues, DiagnoseIssue{
						Severity: q.Severity,
						Class:    "quirk",
						Module:   q.Module,
						Message:  fmt.Sprintf("Quirk '%s': module param '%s' should be '%s' but is not set — %s", q.ID, param, expectedValue, q.Description),
						Fix:      fmt.Sprintf("Add %s=%s to module options. See: %s", param, expectedValue, q.Citation),
						Source:   "quirk_db",
					})
				}
			} else if currentValue != expectedValue {
				result.Issues = append(result.Issues, DiagnoseIssue{
					Severity: q.Severity,
					Class:    "quirk",
					Module:   q.Module,
					Message:  fmt.Sprintf("Quirk '%s': module param '%s' is '%s' but should be '%s' — %s", q.ID, param, currentValue, expectedValue, q.Description),
					Fix:      fmt.Sprintf("Change to %s=%s. See: %s", param, expectedValue, q.Citation),
					Source:   "quirk_db",
				})
			}
		}
	}
}

// scanDmesg looks for errors related to this device in the kernel log
func scanDmesg(dev *probe.Device) []DiagnoseIssue {
	var issues []DiagnoseIssue

	// Use dmesg command (works without root, reads from kernel log buffer)
	cmd := exec.Command("dmesg", "--level=err,warn", "--time-format=reltime")
	output, err := cmd.Output()
	if err != nil {
		// Fallback: try plain dmesg (older kernels without --level)
		cmd = exec.Command("dmesg")
		output, err = cmd.Output()
		if err != nil {
			return issues
		}
	}

	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		// Match by device identifiers
		relevant := false
		if dev.Driver != "" && strings.Contains(strings.ToLower(line), strings.ToLower(dev.Driver)) {
			relevant = true
		}
		if dev.Vendor != "" && dev.DeviceID != "" {
			idStr := dev.Vendor + ":" + dev.DeviceID
			if strings.Contains(line, idStr) {
				relevant = true
			}
		}
		if dev.Description != "" && len(dev.Description) > 3 {
			// Only match on description if it's specific enough (>3 chars)
			if strings.Contains(strings.ToLower(line), strings.ToLower(dev.Description)) {
				relevant = true
			}
		}

		if !relevant {
			continue
		}

		// Check for error patterns
		lowerLine := strings.ToLower(line)
		if strings.Contains(lowerLine, "error") || strings.Contains(lowerLine, "fail") ||
			strings.Contains(lowerLine, "timeout") || strings.Contains(lowerLine, "warning") ||
			strings.Contains(lowerLine, "firmware") || strings.Contains(lowerLine, "not found") ||
			strings.Contains(lowerLine, "refused") || strings.Contains(lowerLine, "invalid") {

			severity := "warning"
			if strings.Contains(lowerLine, "fatal") || strings.Contains(lowerLine, "panic") || strings.Contains(lowerLine, "oops") {
				severity = "fatal"
			}

			issues = append(issues, DiagnoseIssue{
				Severity: severity,
				Class:    "dmesg",
				Module:   dev.Driver,
				Message:  strings.TrimSpace(line),
				Fix:      "Check kernel documentation for this error. May indicate hardware failure, missing firmware, or driver bug.",
				Source:   "dmesg",
			})

			// Limit to 5 dmesg issues per device
			if len(issues) >= 5 {
				break
			}
		}
	}

	return issues
}

// generateSuggestions creates human-readable fix suggestions
func generateSuggestions(result *DiagnoseResult) []string {
	var suggestions []string

	for _, issue := range result.Issues {
		if issue.Fix != "" {
			suggestions = append(suggestions, issue.Fix)
		}
	}

	// Add quirk-related suggestions
	for _, q := range result.Quirks {
		if len(q.Params) > 0 {
			for param, value := range q.Params {
				suggestions = append(suggestions, fmt.Sprintf("Set %s.%s=%s (%s)", q.Module, param, value, q.Description))
			}
		}
	}

	// Deduplicate
	seen := make(map[string]bool)
	var unique []string
	for _, s := range suggestions {
		if !seen[s] {
			seen[s] = true
			unique = append(unique, s)
		}
	}

	return unique
}

// resolveModalias uses modprobe to find the module for a modalias
// Returns the first resolved module
func resolveModalias(modalias string) string {
	if modalias == "" {
		return ""
	}
	cmd := exec.Command("modprobe", "--resolve-alias", modalias)
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(output), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			return line
		}
	}
	return ""
}

// isBlacklisted checks if a module is in any modprobe blacklist
func isBlacklisted(module string) bool {
	blacklistDirs := []string{"/etc/modprobe.d", "/usr/lib/modprobe.d", "/run/modprobe.d"}
	for _, dir := range blacklistDirs {
		files, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, file := range files {
			if file.IsDir() {
				continue
			}
			data, err := os.ReadFile(filepath.Join(dir, file.Name()))
			if err != nil {
				continue
			}
			// Check for "blacklist <module>" lines
			lines := strings.Split(string(data), "\n")
			for _, line := range lines {
				line = strings.TrimSpace(line)
				if strings.HasPrefix(line, "#") {
					continue
				}
				fields := strings.Fields(line)
				if len(fields) >= 2 && fields[0] == "blacklist" && fields[1] == module {
					return true
				}
			}
		}
	}
	return false
}

// findBlacklistSource finds which file blacklists a module
func findBlacklistSource(module string) string {
	blacklistDirs := []string{"/etc/modprobe.d", "/usr/lib/modprobe.d", "/run/modprobe.d"}
	for _, dir := range blacklistDirs {
		files, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, file := range files {
			if file.IsDir() {
				continue
			}
			data, err := os.ReadFile(filepath.Join(dir, file.Name()))
			if err != nil {
				continue
			}
			lines := strings.Split(string(data), "\n")
			for _, line := range lines {
				line = strings.TrimSpace(line)
				if strings.HasPrefix(line, "#") {
					continue
				}
				fields := strings.Fields(line)
				if len(fields) >= 2 && fields[0] == "blacklist" && fields[1] == module {
					return fmt.Sprintf("Remove 'blacklist %s' from %s/%s", module, dir, file.Name())
				}
			}
		}
	}
	return fmt.Sprintf("Check /etc/modprobe.d/ for a 'blacklist %s' entry", module)
}

// checkFirmware checks if required firmware files exist
func checkFirmware(dev *probe.Device, module string) string {
	// Read the module's firmware requirements from sysfs
	fwPath := filepath.Join(dev.Path, "firmware")
	if _, err := os.Stat(fwPath); err != nil {
		return ""
	}
	// Some devices list required firmware in sysfs
	fwDir := filepath.Join(dev.Path, "firmware")
	entries, err := os.ReadDir(fwDir)
	if err != nil {
		return ""
	}
	for _, entry := range entries {
		fwName := entry.Name()
		fwFile := filepath.Join("/lib/firmware", fwName)
		if _, err := os.Stat(fwFile); err != nil {
			return fwName
		}
	}
	return ""
}

// readModuleParam reads a module parameter from /sys/module/<module>/parameters/<param>
func readModuleParam(module, param string) string {
	path := fmt.Sprintf("/sys/module/%s/parameters/%s", module, param)
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// JSON returns the diagnose result as formatted JSON
func (r *DiagnoseResult) JSON() (string, error) {
	data, err := json.MarshalIndent(r, "", "  ")
	return string(data), err
}

// PrintHuman prints the diagnose result in human-readable format
func (r *DiagnoseResult) PrintHuman() string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("Pinscher Diagnose — %s\n", r.Device.Description))
	sb.WriteString(fmt.Sprintf("Bus: %s | Driver: %s | Status: %s\n\n",
		r.Device.Bus, r.Device.Driver, r.Status))

	if len(r.Issues) == 0 && len(r.Quirks) == 0 {
		sb.WriteString("✓ No issues found. Device appears to be working correctly.\n")
		return sb.String()
	}

	if len(r.Issues) > 0 {
		sb.WriteString(fmt.Sprintf("Issues (%d):\n\n", len(r.Issues)))
		for _, issue := range r.Issues {
			icon := "⚠"
			if issue.Severity == "fatal" {
				icon = "✗"
			} else if issue.Severity == "info" {
				icon = "ℹ"
			}
			sb.WriteString(fmt.Sprintf("%s [%s] %s\n", icon, strings.ToUpper(issue.Severity), issue.Message))
			if issue.Fix != "" {
				sb.WriteString(fmt.Sprintf("  Fix: %s\n", issue.Fix))
			}
			sb.WriteString(fmt.Sprintf("  Source: %s\n\n", issue.Source))
		}
	}

	if len(r.Quirks) > 0 {
		sb.WriteString(fmt.Sprintf("Known quirks (%d):\n\n", len(r.Quirks)))
		for _, q := range r.Quirks {
			sb.WriteString(fmt.Sprintf("📋 %s (%s)\n", q.ID, q.Severity))
			sb.WriteString(fmt.Sprintf("  %s\n", q.Description))
			if q.Citation != "" {
				sb.WriteString(fmt.Sprintf("  Ref: %s\n", q.Citation))
			}
			sb.WriteString("\n")
		}
	}

	if len(r.Suggestions) > 0 {
		sb.WriteString("Suggested fixes:\n")
		for i, s := range r.Suggestions {
			sb.WriteString(fmt.Sprintf("  %d. %s\n", i+1, s))
		}
	}

	return sb.String()
}

// JSON returns the diagnose result as formatted JSON
