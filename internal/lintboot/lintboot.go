package lintboot

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/eonicman/pinscher/internal/probe"
)

// BootIssue represents a problem found by the boot linter
type BootIssue struct {
	Severity string `json:"severity"` // "fatal", "warning", "info"
	Class    string `json:"class"`    // "storage", "usb", "gpu", "input", "crypto"
	Device   string `json:"device"`   // device description
	Module   string `json:"module"`   // kernel module that should be in initramfs
	Message  string `json:"message"`  // human-readable explanation
	Fix      string `json:"fix"`      // suggested fix
}

// LintResult is the output of pinscher lint-boot
type LintResult struct {
	Initramfs    string      `json:"initramfs"`
	Kernel       string      `json:"kernel"`
	Issues       []BootIssue `json:"issues"`
	AllClear     bool        `json:"all_clear"`
	CheckedCount int         `json:"checked_count"`

	// Assertions is populated only when --assert was used; nil (omitted from
	// JSON) otherwise, so every existing consumer of lint-boot's output
	// (including the serve daemon's dispatch) sees an unchanged shape.
	// Added by Ghost, 2026-10-17, alongside the --assert flag in main.go.
	Assertions []AssertionResult `json:"assertions,omitempty"`
}

// Boot-critical device classes (PCI class codes)
// These are the devices that MUST have drivers in the initramfs
var bootCriticalClasses = map[string]string{
	"0101":   "storage", // IDE
	"0102":   "storage", // Floppy
	"0104":   "storage", // RAID
	"0105":   "storage", // ATA
	"0106":   "storage", // SATA
	"0107":   "storage", // Serial Attached SCSI
	"0108":   "storage", // NVMe
	"010802": "storage", // NVMe (full class)
	"0c03":   "usb",     // USB Controller (generic)
	"0c0300": "usb",     // UHCI
	"0c0310": "usb",     // OHCI
	"0c0320": "usb",     // EHCI
	"0c0330": "usb",     // xHCI
	"0300":   "gpu",     // VGA
	"030000": "gpu",     // VGA (full class)
	"0302":   "gpu",     // 3D Controller
	"030200": "gpu",     // 3D Controller (full class)
	"0200":   "network", // Ethernet
	"020000": "network", // Ethernet (full class)
	"0280":   "network", // Wireless
	"028000": "network", // Wireless (full class)
	"0801":   "crypto",  // HW Cryptographic device
	"0805":   "input",   // SD Host controller
}

// Boot-critical platform modalias patterns (matched by prefix)
var bootCriticalPlatformModalias = map[string]string{
	"acpi:PNP0C0C":      "input", // Power button
	"acpi:PNP0C0E":      "input", // Sleep button
	"acpi:PNP0C09":      "input", // Embedded controller
	"platform:applesmc": "input", // Apple SMC (fans, sensors, lid)
}

// bootCriticalCategory maps a PCI class code to a boot-critical category
// ("storage", "usb", "gpu", ...). It accepts the class with or without a
// leading "0x" and in either case, tries the 6-digit form (base class +
// subclass + interface) first, then the 4-digit form (base class + subclass).
func bootCriticalCategory(class string) (string, bool) {
	class = probe.NormalizePCIClass(class)
	if len(class) >= 6 {
		if c, ok := bootCriticalClasses[class[:6]]; ok {
			return c, true
		}
	}
	if len(class) >= 4 {
		if c, ok := bootCriticalClasses[class[:4]]; ok {
			return c, true
		}
	}
	return "", false
}

// LintBoot checks that the initramfs contains drivers for all boot-critical hardware
func LintBoot(initramfsPath string) (*LintResult, error) {
	result := &LintResult{
		Initramfs: initramfsPath,
	}

	// Get kernel version
	result.Kernel = readSysFile("/proc/sys/kernel/osrelease")

	// If no path specified, try to find the default initramfs
	if initramfsPath == "" {
		initramfsPath = findInitramfs(result.Kernel)
		if initramfsPath == "" {
			return nil, fmt.Errorf("no initramfs found — specify path explicitly")
		}
		result.Initramfs = initramfsPath
	}

	// 1. Probe all hardware
	probeResult, err := probe.Probe()
	if err != nil {
		return nil, fmt.Errorf("probe failed: %w", err)
	}

	// 2. Get list of modules currently in the initramfs
	initramfsModules, err := listInitramfsModules(initramfsPath)
	if err != nil {
		return nil, fmt.Errorf("cannot read initramfs: %w", err)
	}

	moduleSet := make(map[string]bool)
	for _, m := range initramfsModules {
		moduleSet[m] = true
	}

	// 3. For each boot-critical device, check if its driver is in the initramfs
	// Dedup by module+category (not device) — same module from two GPUs is one issue
	seenModules := make(map[string]bool)

	for _, dev := range probeResult.Devices {
		// --- PCI boot-critical check ---
		if dev.Bus == "pci" {
			// Determine if this device class is boot-critical
			category, critical := bootCriticalCategory(dev.Class)
			if !critical {
				continue
			}

			// Resolve modalias to module(s) — may return multiple
			modules := modulesToCheck(dev.Driver, resolveModaliasAll(dev.Modalias))
			if len(modules) == 0 {
				continue
			}

			for _, module := range modules {
				// Deduplicate by module+category
				dedupKey := module + ":" + category
				if seenModules[dedupKey] {
					continue
				}
				seenModules[dedupKey] = true
				result.CheckedCount++

				// Check if module is in initramfs
				if !moduleSet[module] {
					// Also check for builtin modules (they don't need to be in initramfs)
					if isBuiltinModule(module, result.Kernel) {
						continue // Builtin is fine
					}

					issue := BootIssue{
						Severity: severityForCategory(category),
						Class:    category,
						Device:   dev.Description,
						Module:   module,
						Message:  fmt.Sprintf("%s module '%s' is missing from initramfs — %s may not work at boot", category, module, category),
						Fix:      fixText(module, result.Initramfs),
					}

					// Enhance message for specific categories
					if category == "usb" {
						issue.Message = fmt.Sprintf("USB host controller driver '%s' missing from initramfs — keyboard may be dead at LUKS prompt", module)
						issue.Fix = fixText(module, result.Initramfs)
					} else if category == "storage" {
						issue.Message = fmt.Sprintf("Storage controller driver '%s' missing from initramfs — root filesystem may not mount", module)
						issue.Fix = fixText(module, result.Initramfs)
					}

					result.Issues = append(result.Issues, issue)
				}
			}
		}

		// --- Platform/ACPI boot-critical check ---
		if dev.Bus == "platform" || dev.Bus == "acpi" {
			// Check if the modalias matches a boot-critical pattern
			for pattern, category := range bootCriticalPlatformModalias {
				if strings.HasPrefix(dev.Modalias, pattern) {
					// This is a boot-critical platform/ACPI device
					if dev.Driver != "" {
						// Already has a driver, no issue
						result.CheckedCount++
						break
					}

					// Try to resolve modalias
					modules := resolveModaliasAll(dev.Modalias)
					if len(modules) == 0 {
						break
					}

					for _, module := range modules {
						dedupKey := module + ":" + category
						if seenModules[dedupKey] {
							continue
						}
						seenModules[dedupKey] = true

						if !moduleSet[module] && !isBuiltinModule(module, result.Kernel) {
							issue := BootIssue{
								Severity: "warning",
								Class:    category,
								Device:   dev.Description,
								Module:   module,
								Message:  fmt.Sprintf("Platform driver '%s' for %s is missing from initramfs", module, dev.Description),
								Fix:      fixText(module, result.Initramfs),
							}
							result.Issues = append(result.Issues, issue)
						}
					}
					break // Only match one pattern per device
				}
			}
		}
	}

	// Sort issues by severity
	sort.Slice(result.Issues, func(i, j int) bool {
		return severityRank(result.Issues[i].Severity) > severityRank(result.Issues[j].Severity)
	})

	result.AllClear = len(result.Issues) == 0
	return result, nil
}

// listInitramfsModules extracts and lists modules in an initramfs image
func listInitramfsModules(path string) ([]string, error) {
	// Use lsinitcpio (Arch Linux) or lsinitramfs (Debian) to list modules
	// Try lsinitcpio first (we're on CachyOS/Arch)
	// Plain `lsinitcpio <img>` lists files. `-a` is an analysis mode that prints a
	// summary with no .ko paths, which made every module look missing on
	// Arch/CachyOS (found 2026-10-19).
	cmd := exec.Command("lsinitcpio", path)
	output, err := cmd.Output()
	if err != nil {
		// Try lsinitramfs as fallback
		cmd = exec.Command("lsinitramfs", path)
		output, err = cmd.Output()
		if err != nil {
			return nil, fmt.Errorf("cannot list initramfs contents (tried lsinitcpio and lsinitramfs): %w", err)
		}
	}

	return parseModuleList(string(output)), nil
}

// parseModuleList extracts kernel module names from an initramfs file listing
// (lsinitcpio / lsinitramfs output): any line ending in .ko, .ko.zst, .ko.xz
// or .ko.gz, reduced to the module name with '-' turned into '_'.
func parseModuleList(output string) []string {
	var modules []string
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasSuffix(line, ".ko") || strings.HasSuffix(line, ".ko.zst") || strings.HasSuffix(line, ".ko.xz") || strings.HasSuffix(line, ".ko.gz") {
			base := filepath.Base(line)
			base = strings.TrimSuffix(base, ".ko.zst")
			base = strings.TrimSuffix(base, ".ko.xz")
			base = strings.TrimSuffix(base, ".ko.gz")
			base = strings.TrimSuffix(base, ".ko")
			modules = append(modules, normalizeModuleName(base))
		}
	}
	return modules
}

// resolveModaliasAll returns all modules resolved for a modalias
func resolveModaliasAll(modalias string) []string {
	if modalias == "" {
		return nil
	}
	cmd := exec.Command("modprobe", "--resolve-alias", modalias)
	output, err := cmd.Output()
	if err != nil {
		return nil
	}
	var modules []string
	for _, line := range strings.Split(string(output), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			modules = append(modules, line)
		}
	}
	return modules
}

// isBuiltinModule checks if a module is built into the kernel (not a loadable module)
func isBuiltinModule(module, kernelVersion string) bool {
	builtinPath := fmt.Sprintf("/lib/modules/%s/modules.builtin", kernelVersion)
	data, err := os.ReadFile(builtinPath)
	if err != nil {
		return false
	}
	return moduleInBuiltinList(string(data), module)
}

// moduleInBuiltinList reports whether module appears in the text of a
// modules.builtin file. The kernel treats '-' and '_' in module names as the
// same character, and modules.builtin spells names with dashes where modprobe
// aliases use underscores (ehci-pci.ko vs ehci_pci). It compares whole file
// names, so "ahci" is not matched by "libahci". Found 2026-10-19: a
// substring match on the underscore spelling flagged built-in drivers as
// missing from the initramfs.
func moduleInBuiltinList(builtinList, module string) bool {
	want := normalizeModuleName(module)
	for _, line := range strings.Split(builtinList, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasSuffix(line, ".ko") {
			continue
		}
		if normalizeModuleName(strings.TrimSuffix(filepath.Base(line), ".ko")) == want {
			return true
		}
	}
	return false
}

// normalizeModuleName lowercases a module name and treats '-' as '_'.
func normalizeModuleName(name string) string {
	return strings.ReplaceAll(strings.ToLower(name), "-", "_")
}

// findInitramfs finds the default initramfs for the running kernel.
func findInitramfs(kernelVersion string) string {
	return findInitramfsIn("", kernelVersion)
}

// findInitramfsIn is findInitramfs with a filesystem root prefix, so tests can
// use a temporary directory. root "" means the real filesystem.
//
// Arch-family kernels name the image after the kernel package ("pkgbase"),
// e.g. /boot/initramfs-linux-cachyos.img, not after the version; the first
// real run on CachyOS (Annie, 2026-10-19) found nothing. Debian/Ubuntu paths
// were added after the first run there (fragbox, 2026-10-18).
func findInitramfsIn(root, kernelVersion string) string {
	var candidates []string
	for _, dir := range []string{"/usr/lib/modules", "/lib/modules"} {
		if base := readSysFile(filepath.Join(root, dir, kernelVersion, "pkgbase")); base != "" {
			candidates = append(candidates,
				fmt.Sprintf("/boot/initramfs-%s.img", base),
				fmt.Sprintf("/boot/initramfs-%s-fallback.img", base))
			break
		}
	}
	candidates = append(candidates,
		fmt.Sprintf("/boot/initramfs-%s.img", kernelVersion),
		fmt.Sprintf("/boot/initramfs-%s-fallback.img", kernelVersion),
		"/boot/initramfs-linux.img",
		"/boot/initramfs-linux-fallback.img",
		fmt.Sprintf("/boot/initrd.img-%s", kernelVersion),
		"/boot/initrd.img",
	)
	for _, c := range candidates {
		if _, err := os.Stat(filepath.Join(root, c)); err == nil {
			return filepath.Join(root, c)
		}
	}
	// Last resort: exactly one non-fallback initramfs in /boot.
	matches, _ := filepath.Glob(filepath.Join(root, "/boot/initramfs-*.img"))
	var plain []string
	for _, m := range matches {
		if !strings.Contains(m, "fallback") {
			plain = append(plain, m)
		}
	}
	if len(plain) == 1 {
		return plain[0]
	}
	return ""
}

// modulesToCheck picks the modules that matter for a boot-critical device.
// modprobe --resolve-alias also returns sibling drivers that never bind (for
// example amdgpu for an old Radeon that uses radeon), which produced false
// "missing from initramfs" warnings. If the driver bound at runtime is one of
// the alias matches, check only that one; otherwise (for example xhci_hcd vs
// the xhci_pci module) keep every alias match.
func modulesToCheck(boundDriver string, aliasModules []string) []string {
	if boundDriver == "" {
		return aliasModules
	}
	want := normalizeModuleName(boundDriver)
	for _, m := range aliasModules {
		if normalizeModuleName(m) == want {
			return []string{m}
		}
	}
	return aliasModules
}

// fixText returns the suggested fix for a module missing from the initramfs,
// matched to the tool that built it (initramfs-tools on Debian/Ubuntu,
// mkinitcpio on Arch).
func fixText(module, initramfsPath string) string {
	if strings.Contains(filepath.Base(initramfsPath), "initrd.img") {
		return fmt.Sprintf("Add '%s' to /etc/initramfs-tools/modules and regenerate: sudo update-initramfs -u", module)
	}
	return fmt.Sprintf("Add '%s' to MODULES in /etc/mkinitcpio.conf and regenerate: sudo mkinitcpio -P", module)
}

func severityForCategory(category string) string {
	switch category {
	case "storage", "usb":
		return "fatal"
	case "gpu":
		return "warning"
	case "crypto":
		return "info"
	default:
		return "warning"
	}
}

func severityRank(sev string) int {
	switch sev {
	case "fatal":
		return 3
	case "warning":
		return 2
	case "info":
		return 1
	default:
		return 0
	}
}

func readSysFile(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// JSON returns the lint result as formatted JSON
func (r *LintResult) JSON() (string, error) {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// PrintHuman prints the lint result in human-readable format
func (r *LintResult) PrintHuman() string {
	var sb strings.Builder

	if len(r.Issues) == 0 {
		sb.WriteString(fmt.Sprintf("✓ All clear — %d boot-critical devices checked, initramfs covers all drivers.\n", r.CheckedCount))
	} else {
		sb.WriteString(fmt.Sprintf("Pinscher boot lint — %d devices checked, %d issues found:\n\n", r.CheckedCount, len(r.Issues)))
		for _, issue := range r.Issues {
			icon := "⚠"
			if issue.Severity == "fatal" {
				icon = "✗"
			} else if issue.Severity == "info" {
				icon = "ℹ"
			}
			sb.WriteString(fmt.Sprintf("%s [%s] %s\n", icon, strings.ToUpper(issue.Severity), issue.Message))
			sb.WriteString(fmt.Sprintf("  Fix: %s\n\n", issue.Fix))
		}
	}

	// Assertions (--assert), added by Ghost 2026-10-17: printed regardless of
	// whether boot-critical issues exist, since a clean initramfs and a
	// failed passthrough-binding assertion are independent facts.
	if len(r.Assertions) > 0 {
		failed := 0
		for _, a := range r.Assertions {
			if !a.Pass {
				failed++
			}
		}
		sb.WriteString(fmt.Sprintf("\nAssertions — %d checked, %d failed:\n", len(r.Assertions), failed))
		for _, a := range r.Assertions {
			if a.Pass {
				sb.WriteString(fmt.Sprintf("  ✓ %s:%s=%s\n", a.Field, a.Device, a.Expected))
				continue
			}
			if a.Error != "" {
				sb.WriteString(fmt.Sprintf("  ✗ %s — %s\n", a.Raw, a.Error))
			} else {
				sb.WriteString(fmt.Sprintf("  ✗ %s — got %q, expected %q\n", a.Raw, a.Actual, a.Expected))
			}
		}
	}

	return sb.String()
}

// JSON returns the lint result as formatted JSON
