package synthesize

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/eonicman/pinscher/internal/probe"
	"github.com/eonicman/pinscher/internal/quirk"
)

// SynthesisRung represents the safety ladder level
type SynthesisRung int

const (
	Rung1 SynthesisRung = iota + 1 // Quirk/param/udev rule generation
	Rung2                          // Userspace driver (libusb/uio/vfio)
	Rung3                          // Patched in-tree driver (new PCI IDs, register offsets)
	Rung4                          // Novel kernel module
)

func (r SynthesisRung) String() string {
	switch r {
	case Rung1:
		return "rung-1-quirk-param"
	case Rung2:
		return "rung-2-userspace-driver"
	case Rung3:
		return "rung-3-patched-driver"
	case Rung4:
		return "rung-4-novel-module"
	default:
		return fmt.Sprintf("rung-%d", r)
	}
}

// SynthesizeRequest is the input to the synthesis engine
type SynthesizeRequest struct {
	Device     probe.Device     // the device to synthesize for
	DiagResult *DiagSummary    // optional diagnosis summary
	Rung       SynthesisRung    // which rung to target
	KernelSrc  string           // path to kernel source (for Rung 3+)
	DryRun     bool             // don't apply anything
	Model      string           // Ollama model name (default: llama3.1)
}

// DiagSummary is a lightweight summary of diagnose results for the LLM
type DiagSummary struct {
	Status     string   `json:"status"`
	Driver     string   `json:"driver"`
	Issues     []string `json:"issues"`
	Quirks     []string `json:"quirks"`
	Suggestions []string `json:"suggestions"`
}

// SynthesizeResult is the output of the synthesis engine
type SynthesizeResult struct {
	Device      probe.Device     `json:"device"`
	Rung        SynthesisRung    `json:"rung"`
	Method      string           `json:"method"`       // "llm", "database", "hybrid"
	Confidence  string           `json:"confidence"`   // "high", "medium", "low", "experimental"
	Proposals   []Proposal       `json:"proposals"`
	Warnings    []string         `json:"warnings"`
	RawLLM      string           `json:"raw_llm,omitempty"` // raw LLM response for debugging
}

// Proposal is a single synthesis proposal
type Proposal struct {
	Type        string            `json:"type"`         // "quirk", "module_param", "udev_rule", "firmware", "modprobe_conf", "userspace_driver", "kernel_patch"
	Rung        SynthesisRung     `json:"rung"`
	Title       string            `json:"title"`
	Description string            `json:"description"`
	Action      string            `json:"action"`       // what to do
	Commands    []string          `json:"commands"`     // shell commands to apply (empty if dry-run)
	Files       map[string]string `json:"files,omitempty"` // files to write (path → content)
	Rollback    []string          `json:"rollback"`     // commands to undo
	Reasoning   string            `json:"reasoning"`    // why this proposal
	Citation    string            `json:"citation"`     // source/reference
	Verified    bool              `json:"verified"`     // tested on real hardware?
}

// ollamaRequest/Response for the Ollama API
type ollamaRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	Stream bool   `json:"stream"`
	System string `json:"system,omitempty"`
}

type ollamaResponse struct {
	Response string `json:"response"`
	Done     bool   `json:"done"`
}

// OllamaURL is the default Ollama API endpoint
var OllamaURL = "http://localhost:11434"

// DefaultModel is the default LLM model for synthesis
var DefaultModel = "qwen3:8b"

// Synthesize runs the driver synthesis engine for a device
func Synthesize(req SynthesizeRequest) (*SynthesizeResult, error) {
	result := &SynthesizeResult{
		Device: req.Device,
		Rung:   req.Rung,
	}

	// Step 1: Check quirk database first (always fast, always local)
	dbProposals := synthesizeFromDatabase(req.Device)
	if len(dbProposals) > 0 {
		result.Method = "database"
		result.Proposals = append(result.Proposals, dbProposals...)
	}

	// Step 2: Run diagnose to understand the current state
	diagSummary, err := runDiagnose(req.Device)
	if err == nil {
		req.DiagResult = diagSummary
	}

	// Step 3: Try kernel module resolution
	moduleProposals := synthesizeFromModuleResolution(req.Device)
	if len(moduleProposals) > 0 {
		result.Method = mergeMethod(result.Method, "database")
		result.Proposals = append(result.Proposals, moduleProposals...)
	}

	// Step 4: For Rung 1+, query LLM for synthesis proposals
	if req.Rung >= Rung1 && req.Model != "none" {
		llmProposals, raw, err := synthesizeFromLLM(req)
		if err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("LLM synthesis failed: %v", err))
		} else {
			result.Method = mergeMethod(result.Method, "llm")
			result.Proposals = append(result.Proposals, llmProposals...)
			result.RawLLM = raw
		}
	}

	// Step 5: For Rung 2+, check userspace driver possibilities
	if req.Rung >= Rung2 {
		userspaceProposals := synthesizeUserspace(req.Device)
		if len(userspaceProposals) > 0 {
			result.Method = mergeMethod(result.Method, "database")
			result.Proposals = append(result.Proposals, userspaceProposals...)
		}
	}

	// Step 6: For Rung 3+, check kernel source for similar drivers
	if req.Rung >= Rung3 && req.KernelSrc != "" {
		kernelProposals := synthesizeFromKernelSource(req.Device, req.KernelSrc)
		if len(kernelProposals) > 0 {
			result.Method = mergeMethod(result.Method, "llm")
			result.Proposals = append(result.Proposals, kernelProposals...)
		}
	}

	// Set confidence based on source
	result.Confidence = assessConfidence(result)

	// Add safety warnings
	result.Warnings = append(result.Warnings, safetyWarnings(req.Rung)...)

	// Filter commands if dry-run
	if req.DryRun {
		for i := range result.Proposals {
			result.Proposals[i].Commands = nil
		}
	}

	return result, nil
}

// synthesizeFromDatabase checks the embedded quirk database
func synthesizeFromDatabase(dev probe.Device) []Proposal {
	var proposals []Proposal

	// Check by vendor:device ID
	if dev.Vendor != "" && dev.DeviceID != "" {
		quirks := quirk.FindByDevice(dev.Vendor, dev.DeviceID)
		for _, q := range quirks {
			p := Proposal{
				Type:        "quirk",
				Rung:        Rung1,
				Title:       fmt.Sprintf("Quirk: %s", q.ID),
				Description: q.Description,
				Reasoning:   q.Reason,
				Citation:    q.Citation,
				Verified:    q.Verified,
			}

			// Build action and commands from quirk params
			if len(q.Params) > 0 {
				p.Action = fmt.Sprintf("Set module parameters: %v", q.Params)
				for param, value := range q.Params {
					p.Commands = append(p.Commands, fmt.Sprintf("pinscher set %s %s %s", q.Module, param, value))
					p.Rollback = append(p.Rollback, fmt.Sprintf("pinscher set %s %s <original_value>", q.Module, param))
				}

				// Also propose modprobe.conf entry
				var paramParts []string
				for param, value := range q.Params {
					paramParts = append(paramParts, fmt.Sprintf("%s=%s", param, value))
				}
				p.Files = map[string]string{
					fmt.Sprintf("/etc/modprobe.d/pinscher-%s.conf", q.Module): fmt.Sprintf("# Pinscher quirk: %s\n# Ref: %s\noptions %s %s\n", q.ID, q.Citation, q.Module, strings.Join(paramParts, " ")),
				}
			}

			proposals = append(proposals, p)
		}
	}

	// Check by module name
	if dev.Driver != "" {
		quirks := quirk.FindByModule(dev.Driver)
		for _, q := range quirks {
			// Don't duplicate
			duplicate := false
			for _, existing := range proposals {
				if existing.Title == fmt.Sprintf("Quirk: %s", q.ID) {
					duplicate = true
					break
				}
			}
			if duplicate {
				continue
			}

			p := Proposal{
				Type:        "quirk",
				Rung:        Rung1,
				Title:       fmt.Sprintf("Quirk: %s", q.ID),
				Description: q.Description,
				Reasoning:   q.Reason,
				Citation:    q.Citation,
				Verified:    q.Verified,
			}

			if len(q.Params) > 0 {
				p.Action = fmt.Sprintf("Set module parameters: %v", q.Params)
				for param, value := range q.Params {
					p.Commands = append(p.Commands, fmt.Sprintf("pinscher set %s %s %s", q.Module, param, value))
				}
			}

			proposals = append(proposals, p)
		}
	}

	return proposals
}

// synthesizeFromModuleResolution tries modprobe resolution for devices without drivers
func synthesizeFromModuleResolution(dev probe.Device) []Proposal {
	if dev.Driver != "" || dev.Modalias == "" {
		return nil
	}

	// Try modprobe --resolve-alias
	cmd := exec.Command("modprobe", "--resolve-alias", dev.Modalias)
	output, err := cmd.Output()
	if err != nil {
		return nil
	}

	modules := strings.Split(strings.TrimSpace(string(output)), "\n")
	var proposals []Proposal

	for _, mod := range modules {
		mod = strings.TrimSpace(mod)
		if mod == "" || mod == dev.Modalias {
			continue
		}

		p := Proposal{
			Type:        "module_param",
			Rung:        Rung1,
			Title:       fmt.Sprintf("Load kernel module: %s", mod),
			Description: fmt.Sprintf("The modalias '%s' resolves to module '%s' which is not currently loaded", dev.Modalias, mod),
			Action:      fmt.Sprintf("sudo modprobe %s", mod),
			Commands:    []string{fmt.Sprintf("sudo modprobe %s", mod)},
			Rollback:    []string{fmt.Sprintf("sudo rmmod %s", mod)},
			Reasoning:   fmt.Sprintf("Device modalias %s has a matching kernel module (%s) that isn't loaded. It may need to be added to initramfs or /etc/modules-load.d/.", dev.Modalias, mod),
			Citation:    "pinscher:modprobe-resolve",
			Verified:    false,
		}

		// Check if it's blacklisted
		if isBlacklisted(mod) {
			p.Title = fmt.Sprintf("Unblacklist and load: %s", mod)
			p.Description = fmt.Sprintf("Module '%s' is blacklisted. Remove the blacklist entry and load it.", mod)
			p.Reasoning = fmt.Sprintf("Device modalias %s resolves to module '%s', but it's blacklisted. Check /etc/modprobe.d/ for the blacklist entry.", dev.Modalias, mod)
			p.Reasoning = fmt.Sprintf("Module '%s' is blacklisted. Check /etc/modprobe.d/ for the blacklist entry.", mod)
		}

		// Check if it needs to be in initramfs
		if isBootCritical(mod) {
			p.Files = map[string]string{
				fmt.Sprintf("/etc/modules-load.d/%s.conf", mod): fmt.Sprintf("# Pinscher: auto-loaded for %s\n%s\n", dev.Description, mod),
			}
			p.Reasoning += " This is a boot-critical device — add to initramfs via mkinitcpio or modules-load.d."
		}

		proposals = append(proposals, p)
	}

	return proposals
}

// synthesizeFromLLM queries Ollama for synthesis proposals
func synthesizeFromLLM(req SynthesizeRequest) ([]Proposal, string, error) {
	model := req.Model
	if model == "" {
		model = DefaultModel
	}

	// Build the prompt with device context
	prompt := buildLLMPrompt(req)

	systemPrompt := `You are a Linux hardware driver synthesis expert. You analyze device information and propose driver solutions following a safety ladder:

Rung 1 (safe): Module parameters, sysfs writes, udev rules, firmware placement. Wrong answers fail loudly and reversibly.
Rung 2 (contained): Userspace drivers using libusb/uio/vfio-pci. A crash kills a process, not the kernel.
Rung 3 (risky): Patched in-tree drivers — new device IDs, register-offset tweaks. Requires test partition.
Rung 4 (last resort): Novel kernel modules. Requires sandbox + boot safety.

Respond in JSON format with a "proposals" array. Each proposal must have:
- type: "quirk" | "module_param" | "udev_rule" | "firmware" | "modprobe_conf" | "userspace_driver" | "kernel_patch"
- rung: 1-4
- title: short description
- description: what it does
- action: one-line summary of what to do
- commands: array of shell commands to apply
- rollback: array of shell commands to undo
- files: object mapping file paths to file content (for config files)
- reasoning: why this proposal is correct
- citation: URL or reference if known
- confidence: "high" | "medium" | "low" | "experimental"

Be conservative. Prefer Rung 1 solutions. If you're uncertain, say so. Never hallucinate kernel module names or register offsets.`

	ollamaReq := ollamaRequest{
		Model:  model,
		Prompt: prompt,
		Stream: false,
		System: systemPrompt,
	}

	body, err := json.Marshal(ollamaReq)
	if err != nil {
		return nil, "", fmt.Errorf("marshal request: %w", err)
	}

	client := &http.Client{Timeout: 300 * time.Second}
	resp, err := client.Post(OllamaURL+"/api/generate", "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, "", fmt.Errorf("ollama request failed: %w (is ollama running?)", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("ollama returned status %d", resp.StatusCode)
	}

	// Read full response (Ollama may stream, but we set stream=false)
	var fullResponse strings.Builder
	decoder := json.NewDecoder(resp.Body)
	for decoder.More() {
		var chunk ollamaResponse
		if err := decoder.Decode(&chunk); err != nil {
			break
		}
		fullResponse.WriteString(chunk.Response)
	}

	raw := fullResponse.String()

	// Parse the LLM response — it may contain markdown code fences
	proposals := parseLLMProposals(raw)

	// Set rung from request
	for i := range proposals {
		if proposals[i].Rung == 0 {
			proposals[i].Rung = req.Rung
		}
	}

	return proposals, raw, nil
}

// buildLLMPrompt constructs the prompt for the LLM
func buildLLMPrompt(req SynthesizeRequest) string {
	var sb strings.Builder

	sb.WriteString("## Device Information\n\n")
	sb.WriteString(fmt.Sprintf("Bus: %s\n", req.Device.Bus))
	sb.WriteString(fmt.Sprintf("Description: %s\n", req.Device.Description))
	sb.WriteString(fmt.Sprintf("Vendor: %s\n", req.Device.Vendor))
	sb.WriteString(fmt.Sprintf("Device ID: %s\n", req.Device.DeviceID))
	sb.WriteString(fmt.Sprintf("Class: %s (%s)\n", req.Device.Class, req.Device.ClassName))
	sb.WriteString(fmt.Sprintf("Modalias: %s\n", req.Device.Modalias))
	sb.WriteString(fmt.Sprintf("Current driver: %s\n", driverStatus(req.Device.Driver)))
	sb.WriteString(fmt.Sprintf("Path: %s\n", req.Device.Path))

	if len(req.Device.Capabilities) > 0 {
		sb.WriteString("\nCapabilities:\n")
		for k, v := range req.Device.Capabilities {
			sb.WriteString(fmt.Sprintf("  %s: %s\n", k, v))
		}
	}

	if req.DiagResult != nil {
		sb.WriteString("\n## Diagnosis\n\n")
		sb.WriteString(fmt.Sprintf("Status: %s\n", req.DiagResult.Status))
		if req.DiagResult.Driver != "" {
			sb.WriteString(fmt.Sprintf("Driver: %s\n", req.DiagResult.Driver))
		}
		for _, issue := range req.DiagResult.Issues {
			sb.WriteString(fmt.Sprintf("Issue: %s\n", issue))
		}
		for _, q := range req.DiagResult.Quirks {
			sb.WriteString(fmt.Sprintf("Quirk: %s\n", q))
		}
		for _, s := range req.DiagResult.Suggestions {
			sb.WriteString(fmt.Sprintf("Suggestion: %s\n", s))
		}
	}

	sb.WriteString(fmt.Sprintf("\n## Target Rung: %d (%s)\n", req.Rung, req.Rung))
	sb.WriteString("\nPropose driver synthesis solutions at this rung level or below. Prefer the lowest safe rung.\n")

	return sb.String()
}

// parseLLMProposals attempts to parse LLM output into Proposal structs
func parseLLMProposals(raw string) []Proposal {
	// Try to extract JSON from the response
	jsonStr := raw

	// Strip markdown code fences if present
	if idx := strings.Index(raw, "```json"); idx != -1 {
		jsonStr = raw[idx+7:]
		if end := strings.Index(jsonStr, "```"); end != -1 {
			jsonStr = jsonStr[:end]
		}
	} else if idx := strings.Index(raw, "```"); idx != -1 {
		jsonStr = raw[idx+3:]
		if end := strings.Index(jsonStr, "```"); end != -1 {
			jsonStr = jsonStr[:end]
		}
	}

	jsonStr = strings.TrimSpace(jsonStr)

	// Try to parse as JSON object with "proposals" array
	var wrapper struct {
		Proposals []Proposal `json:"proposals"`
	}
	if err := json.Unmarshal([]byte(jsonStr), &wrapper); err == nil {
		return wrapper.Proposals
	}

	// Try to parse as JSON array directly
	var proposals []Proposal
	if err := json.Unmarshal([]byte(jsonStr), &proposals); err == nil {
		return proposals
	}

	// If JSON parsing fails, create a single proposal from the raw text
	// This handles cases where the LLM doesn't output valid JSON
	return []Proposal{
		{
			Type:        "llm_suggestion",
			Rung:        Rung1,
			Title:       "LLM Analysis (unstructured)",
			Description: "The LLM provided analysis that couldn't be parsed as structured proposals. See reasoning field for details.",
			Reasoning:   raw,
			Citation:    "pinscher:llm-analysis",
			Verified:    false,
		},
	}
}

// synthesizeUserspace checks if a userspace driver approach is viable
func synthesizeUserspace(dev probe.Device) []Proposal {
	var proposals []Proposal

	switch dev.Bus {
	case "usb":
		// USB devices can use libusb
		if dev.Vendor != "" && dev.DeviceID != "" {
			p := Proposal{
				Type:        "userspace_driver",
				Rung:        Rung2,
				Title:       fmt.Sprintf("Userspace USB driver for %s:%s", dev.Vendor, dev.DeviceID),
				Description: fmt.Sprintf("Device %s (USB %s:%s) could be driven by a userspace driver using libusb or python-usb", dev.Description, dev.Vendor, dev.DeviceID),
				Action:      "Create a userspace driver using libusb",
				Reasoning:   "USB devices without kernel drivers can often be controlled entirely from userspace using libusb. This is safer than a kernel module — a userspace crash doesn't affect the kernel.",
				Citation:    "https://libusb.info/; pinscher:userspace-usb",
				Verified:    false,
			}
			proposals = append(proposals, p)
		}

	case "pci":
		// PCI devices can use VFIO or UIO for userspace access
		if dev.Vendor != "" && dev.DeviceID != "" {
			p := Proposal{
				Type:        "userspace_driver",
				Rung:        Rung2,
				Title:       fmt.Sprintf("VFIO passthrough for %s:%s", dev.Vendor, dev.DeviceID),
				Description: fmt.Sprintf("Device %s (PCI %s:%s) could be passed through to userspace via vfio-pci", dev.Description, dev.Vendor, dev.DeviceID),
				Action:      fmt.Sprintf("Bind device to vfio-pci: echo '%s %s' > /etc/modprobe.d/vfio.conf && sudo modprobe vfio-pci", dev.Vendor, dev.DeviceID),
				Commands: []string{
					fmt.Sprintf("echo 'options vfio-pci ids=%s:%s' | sudo tee /etc/modprobe.d/vfio-%s%s.conf", dev.Vendor, dev.DeviceID, dev.Vendor, dev.DeviceID),
					"sudo modprobe vfio-pci",
				},
				Rollback: []string{
					fmt.Sprintf("sudo rm /etc/modprobe.d/vfio-%s%s.conf", dev.Vendor, dev.DeviceID),
					fmt.Sprintf("sudo modprobe %s", dev.Driver),
				},
				Reasoning: "VFIO allows direct device access from userspace. Useful for custom hardware, FPGA programming, or when writing a kernel driver is impractical.",
				Citation:  "https://kernel.org/doc/Documentation/vfio.txt; pinscher:vfio-passthrough",
				Verified:  false,
			}
			proposals = append(proposals, p)
		}
	}

	return proposals
}

// synthesizeFromKernelSource searches the kernel source tree for similar drivers
func synthesizeFromKernelSource(dev probe.Device, kernelSrc string) []Proposal {
	var proposals []Proposal

	// Check if kernel source exists
	if _, err := os.Stat(kernelSrc); err != nil {
		return proposals
	}

	// Search for the vendor ID in driver source
	if dev.Vendor != "" && dev.Bus == "pci" {
		searchPatterns := []string{
			dev.Vendor,
		}
		if dev.DeviceID != "" {
			searchPatterns = append(searchPatterns, dev.Vendor+dev.DeviceID)
		}

		for _, pattern := range searchPatterns {
			cmd := exec.Command("grep", "-r", "-l", pattern, kernelSrc+"/drivers/")
			output, err := cmd.Output()
			if err != nil {
				continue
			}

			files := strings.Split(strings.TrimSpace(string(output)), "\n")
			for _, file := range files {
				if file == "" {
					continue
				}
				// Extract just the relative path from drivers/
				relPath := strings.TrimPrefix(file, kernelSrc+"/")

				p := Proposal{
					Type:        "kernel_patch",
					Rung:        Rung3,
					Title:       fmt.Sprintf("Kernel source match: %s", relPath),
					Description: fmt.Sprintf("Found vendor ID %s in kernel source file %s. This driver may support this device with minor modifications.", pattern, relPath),
					Action:      fmt.Sprintf("Review %s for device ID table additions", relPath),
					Reasoning:   fmt.Sprintf("The kernel source contains a reference to vendor ID %s in %s. Adding the device ID %s to this driver's PCI ID table may enable support.", dev.Vendor, relPath, dev.DeviceID),
					Citation:    fmt.Sprintf("kernel-source:%s", relPath),
					Verified:    false,
				}
				proposals = append(proposals, p)

				// Limit to 5 proposals from kernel source
				if len(proposals) >= 5 {
					return proposals
				}
			}
		}
	}

	return proposals
}

// runDiagnose runs a lightweight diagnosis for the device
func runDiagnose(dev probe.Device) (*DiagSummary, error) {
	summary := &DiagSummary{
		Status: "unknown",
	}

	if dev.Driver != "" {
		summary.Status = "driver_loaded"
		summary.Driver = dev.Driver
	} else {
		summary.Status = "no_driver"
	}

	// Check quirks
	if dev.Vendor != "" && dev.DeviceID != "" {
		quirks := quirk.FindByDevice(dev.Vendor, dev.DeviceID)
		for _, q := range quirks {
			summary.Quirks = append(summary.Quirks, fmt.Sprintf("%s: %s", q.ID, q.Description))
		}
	}
	if dev.Driver != "" {
		quirks := quirk.FindByModule(dev.Driver)
		for _, q := range quirks {
			summary.Quirks = append(summary.Quirks, fmt.Sprintf("%s: %s", q.ID, q.Description))
		}
	}

	// Check modalias resolution
	if dev.Modalias != "" && dev.Driver == "" {
		cmd := exec.Command("modprobe", "--resolve-alias", dev.Modalias)
		if output, err := cmd.Output(); err == nil {
			module := strings.TrimSpace(string(output))
			if module != "" {
				summary.Suggestions = append(summary.Suggestions, fmt.Sprintf("Module %s resolves for this device but isn't loaded", module))
			}
		}
	}

	return summary, nil
}

// Helper functions

func driverStatus(driver string) string {
	if driver == "" {
		return "(none — no driver loaded)"
	}
	return driver
}

func mergeMethod(existing, newMethod string) string {
	if existing == "" {
		return newMethod
	}
	if existing == newMethod {
		return existing
	}
	return "hybrid"
}

func assessConfidence(result *SynthesizeResult) string {
	hasVerified := false
	hasLLM := false
	hasDatabase := false

	for _, p := range result.Proposals {
		if p.Verified {
			hasVerified = true
		}
		if p.Type == "llm_suggestion" || p.Citation == "pinscher:llm-analysis" {
			hasLLM = true
		}
		if p.Citation != "" && !strings.HasPrefix(p.Citation, "pinscher:") {
			hasDatabase = true
		}
	}

	switch {
	case hasVerified && hasDatabase:
		return "high"
	case hasDatabase:
		return "medium"
	case hasLLM && !hasDatabase:
		return "low"
	default:
		return "experimental"
	}
}

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
			data, err := os.ReadFile(dir + "/" + file.Name())
			if err != nil {
				continue
			}
			for _, line := range strings.Split(string(data), "\n") {
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

func isBootCritical(module string) bool {
	bootCritical := []string{
		"xhci_hcd", "ehci_hcd", "ohci_hcd", "uhci_hcd",
		"nvme", "ahci", "sdhci", "mmc_core",
		"ext4", "btrfs", "xfs", "f2fs",
		"dm_crypt", "luks", "encrypt",
		"hid", "hid_generic", "usbhid",
		"atkbd", "i8042",
		"amdgpu", "nouveau", "i915",
		"efivarfs",
	}
	for _, m := range bootCritical {
		if module == m {
			return true
		}
	}
	return false
}

func safetyWarnings(rung SynthesisRung) []string {
	warnings := []string{
		"Always test driver changes on a non-production system first.",
	}

	if rung >= Rung2 {
		warnings = append(warnings, "Userspace drivers (Rung 2) are safer than kernel modules but still require testing.")
	}

	if rung >= Rung3 {
		warnings = append(warnings, "Kernel patches (Rung 3) can cause system instability. Use a test partition with systemd-boot boot counting.")
		warnings = append(warnings, "Before applying: ensure a rollback boot entry exists and boot counting is enabled.")
	}

	if rung >= Rung4 {
		warnings = append(warnings, "Novel kernel modules (Rung 4) are HIGH RISK. A buggy module can cause kernel panics, data loss, or hardware damage.")
		warnings = append(warnings, "NEVER insmod a novel module on a production system. Use a sacrificial boot environment with boot-counting rollback.")
	}

	return warnings
}

// PrintHuman renders the result in human-readable format
func (r *SynthesizeResult) PrintHuman() string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("Pinscher Synthesize — %s\n", r.Device.Description))
	sb.WriteString(fmt.Sprintf("Bus: %s | Vendor: %s | Device: %s\n", r.Device.Bus, r.Device.Vendor, r.Device.DeviceID))
	sb.WriteString(fmt.Sprintf("Target Rung: %d (%s) | Method: %s | Confidence: %s\n\n",
		r.Rung, r.Rung, r.Method, r.Confidence))

	if len(r.Warnings) > 0 {
		sb.WriteString("⚠ Warnings:\n")
		for _, w := range r.Warnings {
			sb.WriteString(fmt.Sprintf("  • %s\n", w))
		}
		sb.WriteString("\n")
	}

	if len(r.Proposals) == 0 {
		sb.WriteString("No synthesis proposals found. The device may be too obscure or the LLM may not have training data for it.\n")
		sb.WriteString("\nSuggestions:\n")
		sb.WriteString("  • Try a different model with --model\n")
		sb.WriteString("  • Search online for the vendor:device ID\n")
		sb.WriteString("  • Check linux-hardware.org for user reports\n")
		return sb.String()
	}

	sb.WriteString(fmt.Sprintf("Proposals (%d):\n\n", len(r.Proposals)))
	for i, p := range r.Proposals {
		icon := "📋"
		switch p.Rung {
		case Rung1:
			icon = "✓"
		case Rung2:
			icon = "⚡"
		case Rung3:
			icon = "⚠"
		case Rung4:
			icon = "🔴"
		}

		sb.WriteString(fmt.Sprintf("%d. %s [Rung %d] %s\n", i+1, icon, p.Rung, p.Title))
		sb.WriteString(fmt.Sprintf("   Type: %s\n", p.Type))
		sb.WriteString(fmt.Sprintf("   %s\n", p.Description))
		if p.Reasoning != "" {
			sb.WriteString(fmt.Sprintf("   Reasoning: %s\n", p.Reasoning))
		}
		if p.Action != "" {
			sb.WriteString(fmt.Sprintf("   Action: %s\n", p.Action))
		}
		if len(p.Commands) > 0 {
			sb.WriteString("   Commands:\n")
			for _, cmd := range p.Commands {
				sb.WriteString(fmt.Sprintf("     $ %s\n", cmd))
			}
		}
		if len(p.Files) > 0 {
			sb.WriteString("   Files:\n")
			for path, content := range p.Files {
				sb.WriteString(fmt.Sprintf("     → %s\n", path))
				for _, line := range strings.Split(content, "\n") {
					sb.WriteString(fmt.Sprintf("       %s\n", line))
				}
			}
		}
		if len(p.Rollback) > 0 {
			sb.WriteString("   Rollback:\n")
			for _, cmd := range p.Rollback {
				sb.WriteString(fmt.Sprintf("     $ %s\n", cmd))
			}
		}
		if p.Citation != "" {
			sb.WriteString(fmt.Sprintf("   Ref: %s\n", p.Citation))
		}
		if p.Verified {
			sb.WriteString("   ✓ Verified on real hardware\n")
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

// JSON returns the result as formatted JSON
func (r *SynthesizeResult) JSON() (string, error) {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}