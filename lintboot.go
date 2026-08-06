package lintboot

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
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
}

// Boot-critical device classes (PCI class codes)
// These are the devices that MUST have drivers in the initramfs
var bootCriticalClasses = map[string]string{
	"0101":  "storage",  // IDE
	"0102":  "storage",  // Floppy
	"0104":  "storage",  // RAID
	"0105":  "storage",  // ATA
	"0106":  "storage",  // SATA
	"0107":  "storage",  // Serial Attached SCSI
	"0108":  "storage",  // NVMe
	"010802": "storage",  // NVMe (full class)
	"0c03":  "usb",      // USB Controller (generic)
	"0c0300": "usb",     // UHCI
	"0c0310": "usb",     // OHCI
	"0c0320": "usb",     // EHCI
	"0c0330": "usb",     // xHCI
	"0300":  "gpu",      // VGA
	"030000": "gpu",     // VGA (full class)
	"0302":  "gpu",      // 3D Controller
	"030200": "gpu",     // 3D Controller (full class)
	"0200":  "network",  // Ethernet
	"020000": "network", // Ethernet (full class)
	"0280":  "network",  // Wireless
	"028000": "network", // Wireless (full class)
	"0801":  "crypto",   // HW Cryptographic device
	"0805":  "input",    // SD Host controller
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
	checkedDevices := make(map[string]bool)

	for _, dev := range probeResult.Devices {
		if dev.Bus != "pci" {
			continue
		}

		// Determine if this device class is boot-critical
		// PCI class format: bcXXscXXppXX (8 hex chars, e.g. 0c0330)
		classCode := dev.Class
		if len(classCode) >= 6 {
			classCode = classCode[:6] // bcXXsc (base class + subclass)
		}

		category, critical := bootCriticalClasses[classCode]
		if !critical {
			// Also try 4-char prefix (bcXX)
			if len(dev.Class) >= 4 {
				classCode = dev.Class[:4]
				category, critical = bootCriticalClasses[classCode]
			}
		}
		if !critical {
			continue
		}

		// Resolve modalias to module name
		module := resolveModalias(dev.Modalias)
		if module == "" {
			// No module — skip if it's not expected to have one
			continue
		}

		// Deduplicate by module
		key := module + ":" + dev.Path
		if checkedDevices[key] {
			continue
		}
		checkedDevices[key] = true
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
				Fix:      fmt.Sprintf("Add '%s' to MODULES in /etc/mkinitcpio.conf and regenerate: sudo mkinitcpio -P", module),
			}

			// Enhance message for specific categories
			if category == "usb" {
				issue.Message = fmt.Sprintf("USB host controller driver '%s' missing from initramfs — keyboard may be dead at LUKS prompt", module)
				issue.Fix = fmt.Sprintf("Add '%s' to MODULES in /etc/mkinitcpio.conf and regenerate: sudo mkinitcpio -P", module)
			} else if category == "storage" {
				issue.Message = fmt.Sprintf("Storage controller driver '%s' missing from initramfs — root filesystem may not mount", module)
				issue.Fix = fmt.Sprintf("Add '%s' to MODULES in /etc/mkinitcpio.conf and regenerate: sudo mkinitcpio -P", module)
			}

			result.Issues = append(result.Issues, issue)
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
	cmd := exec.Command("lsinitcpio", "-a", path)
	output, err := cmd.Output()
	if err != nil {
		// Try lsinitramfs as fallback
		cmd = exec.Command("lsinitramfs", path)
		output, err = cmd.Output()
		if err != nil {
			return nil, fmt.Errorf("cannot list initramfs contents (tried lsinitcpio and lsinitramfs): %w", err)
		}
	}

	var modules []string
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		// Look for .ko files in the initramfs
		if strings.HasSuffix(line, ".ko") || strings.HasSuffix(line, ".ko.zst") || strings.HasSuffix(line, ".ko.xz") || strings.HasSuffix(line, ".ko.gz") {
			// Extract module name from path
			base := filepath.Base(line)
			// Remove .ko and compression extension
			base = strings.TrimSuffix(base, ".ko.zst")
			base = strings.TrimSuffix(base, ".ko.xz")
			base = strings.TrimSuffix(base, ".ko.gz")
			base = strings.TrimSuffix(base, ".ko")
			// Replace dashes with underscores (kernel module naming)
			base = strings.ReplaceAll(base, "-", "_")
			modules = append(modules, base)
		}
	}

	return modules, nil
}

// resolveModalias uses modprobe --resolve-alias to find the module for a modalias
func resolveModalias(modalias string) string {
	if modalias == "" {
		return ""
	}
	cmd := exec.Command("modprobe", "--resolve-alias", modalias)
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

// isBuiltinModule checks if a module is built into the kernel (not a loadable module)
func isBuiltinModule(module, kernelVersion string) bool {
	builtinPath := fmt.Sprintf("/lib/modules/%s/modules.builtin", kernelVersion)
	data, err := os.ReadFile(builtinPath)
	if err != nil {
		return false
	}
	// Check if module appears in the builtin list
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		if strings.Contains(line, module+".ko") {
			return true
		}
	}
	return false
}

// findInitramfs finds the default initramfs for the running kernel
func findInitramfs(kernelVersion string) string {
	// Try standard Arch Linux locations
	candidates := []string{
		fmt.Sprintf("/boot/initramfs-%s.img", kernelVersion),
		fmt.Sprintf("/boot/initramfs-%s-fallback.img", kernelVersion),
		"/boot/initramfs-linux.img",
		"/boot/initramfs-linux-fallback.img",
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	return ""
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
	if r.AllClear {
		return fmt.Sprintf("✓ All clear — %d boot-critical devices checked, initramfs covers all drivers.\n", r.CheckedCount)
	}

	var sb strings.Builder
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
	return sb.String()
}

// Compile-time regex to suppress unused warning
var _ = regexp.MustCompile