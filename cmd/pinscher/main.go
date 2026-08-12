package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/eonicman/pinscher/internal/config"
	"github.com/eonicman/pinscher/internal/diagnose"
	"github.com/eonicman/pinscher/internal/lintboot"
	"github.com/eonicman/pinscher/internal/probe"
	"github.com/eonicman/pinscher/internal/quirk"
	"github.com/eonicman/pinscher/internal/synthesize"
	"github.com/eonicman/pinscher/internal/watch"
)

const version = "0.6.0"

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	// Global flags that don't need a subcommand
	switch os.Args[1] {
	case "--version", "-V":
		fmt.Printf("pinscher %s\n", version)
		os.Exit(0)
	}

	switch os.Args[1] {
	case "report":
		cmdReport()
	case "lint-boot":
		cmdLintBoot()
	case "diagnose":
		cmdDiagnose()
	case "set":
		cmdSet()
	case "get":
		cmdGet()
	case "list-params":
		cmdListParams()
	case "quirks":
		cmdQuirks()
	case "audit-log":
		cmdAuditLog()
	case "synthesize":
		cmdSynthesize()
	case "apply":
		cmdApply()
	case "watch":
		cmdWatch()
	case "diff":
		cmdDiff()
	case "version":
		fmt.Printf("pinscher %s\n", version)
	case "help", "--help", "-h":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func cmdReport() {
	result, err := probe.Probe()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	jsonOutput := hasFlag("--json", "-j")
	if jsonOutput {
		out, err := result.JSON()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(out)
	} else {
		printReportHuman(result)
	}
}

func cmdLintBoot() {
	initramfsPath := ""
	for _, arg := range os.Args[2:] {
		if arg == "--json" || arg == "-j" {
		} else if !strings.HasPrefix(arg, "-") {
			initramfsPath = arg
		}
	}

	result, err := lintboot.LintBoot(initramfsPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	jsonOutput := hasFlag("--json", "-j")
	if jsonOutput {
		out, err := result.JSON()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(out)
	} else {
		fmt.Print(result.PrintHuman())
		if !result.AllClear {
			os.Exit(1)
		}
	}
}

func printReportHuman(r *probe.ProbeResult) {
	fmt.Printf("Pinscher Hardware Report \u2014 %s\n", r.Hostname)
	fmt.Printf("Kernel: %s\n\n", r.Kernel)
	fmt.Printf("Devices: %d total (%d with driver, %d without)\n\n",
		r.Summary.Total, r.Summary.WithDriver, r.Summary.NoDriver)
	for _, dev := range r.Devices {
		icon := "\u2713"
		if dev.Driver == "" {
			icon = "\u2717"
		}
		desc := dev.Description
		if desc == "" {
			desc = dev.Vendor + ":" + dev.DeviceID
		}
		fmt.Printf("  %s [%s] %s", icon, dev.Bus, desc)
		if dev.Driver != "" {
			fmt.Printf(" (%s)", dev.Driver)
		} else {
			fmt.Printf(" \u2014 NO DRIVER")
		}
		fmt.Println()
	}
}

func cmdDiagnose() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "Usage: pinscher diagnose <device> [--json]")
		fmt.Fprintln(os.Stderr, "  device can be: PCI address, vendor:device ID, driver name, or description")
		os.Exit(1)
	}

	deviceQuery := os.Args[2]
	jsonOutput := hasFlag("--json", "-j")

	result, err := diagnose.Diagnose(deviceQuery)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if jsonOutput {
		data, _ := json.MarshalIndent(result, "", "  ")
		fmt.Println(string(data))
	} else {
		fmt.Print(result.PrintHuman())
		if len(result.Issues) > 0 {
			for _, issue := range result.Issues {
				if issue.Severity == "fatal" {
					os.Exit(1)
				}
			}
		}
	}
}

func cmdSet() {
	if len(os.Args) < 5 {
		fmt.Fprintln(os.Stderr, "Usage: pinscher set <module> <param> <value> [--dry-run]")
		os.Exit(1)
	}

	module := os.Args[2]
	param := os.Args[3]
	value := os.Args[4]
	dryRun := hasFlag("--dry-run")

	op, err := config.SetParam(module, param, value, dryRun)
	if err != nil {
		fmt.Fprintf(os.Stderr, "✗ %v\n", err)
		if op != nil {
			fmt.Fprintf(os.Stderr, "  Module: %s.%s\n", op.Module, op.Param)
			fmt.Fprintf(os.Stderr, "  Current: %s → Requested: %s\n", op.OldValue, op.NewValue)
		}
		os.Exit(1)
	}

	if dryRun {
		fmt.Printf("DRY RUN — %s.%s: %s → %s (no changes made)\n", module, param, op.OldValue, value)
	} else {
		fmt.Printf("✓ %s.%s: %s → %s\n", module, param, op.OldValue, value)
	}
}

func cmdGet() {
	if len(os.Args) < 4 {
		fmt.Fprintln(os.Stderr, "Usage: pinscher get <module> <param>")
		os.Exit(1)
	}

	module := os.Args[2]
	param := os.Args[3]

	value, err := config.GetParam(module, param)
	if err != nil {
		fmt.Fprintf(os.Stderr, "✗ %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("%s.%s = %s\n", module, param, value)
}

func cmdListParams() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "Usage: pinscher list-params <module>")
		os.Exit(1)
	}

	module := os.Args[2]
	params, err := config.ListParams(module)
	if err != nil {
		fmt.Fprintf(os.Stderr, "✗ %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Parameters for %s (%d):\n", module, len(params))
	for _, p := range params {
		value, _ := config.GetParam(module, p)
		fmt.Printf("  %s = %s\n", p, value)
	}
}

func cmdQuirks() {
	jsonOutput := hasFlag("--json", "-j")

	if len(os.Args) >= 3 && os.Args[2] != "--json" && os.Args[2] != "-j" {
		// Filter by module or device
		query := os.Args[2]
		quirks := quirk.FindByModule(query)
		if len(quirks) == 0 {
			// Try as device ID
			parts := strings.Split(query, ":")
			if len(parts) == 2 {
				quirks = quirk.FindByDevice(parts[0], parts[1])
			}
		}
		if jsonOutput {
			data, _ := json.MarshalIndent(quirks, "", "  ")
			fmt.Println(string(data))
		} else {
			printQuirksHuman(quirks)
		}
	} else {
		all := quirk.FindAll()
		if jsonOutput {
			data, _ := json.MarshalIndent(all, "", "  ")
			fmt.Println(string(data))
		} else {
			printQuirksHuman(all)
		}
	}
}

func cmdAuditLog() {
	fmt.Print(config.PrintAuditLog())
}

func cmdSynthesize() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "Usage: pinscher synthesize <device> [options]")
		fmt.Fprintln(os.Stderr, "  device can be: PCI address, vendor:device ID, driver name, or description")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Options:")
		fmt.Fprintln(os.Stderr, "  --json, -j          Output as JSON")
		fmt.Fprintln(os.Stderr, "  --rung N            Target synthesis rung (1-4, default: 1)")
		fmt.Fprintln(os.Stderr, "  --model MODEL       Ollama model to use (default: qwen3:8b, 'none' to skip LLM)")
		fmt.Fprintln(os.Stderr, "  --kernel-src PATH   Path to kernel source tree (for Rung 3+)")
		fmt.Fprintln(os.Stderr, "  --dry-run           Don't apply any changes")
		fmt.Fprintln(os.Stderr, "  --ollama-url URL    Ollama API URL (default: http://localhost:11434)")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Rungs:")
		fmt.Fprintln(os.Stderr, "  1  Quirk/param/udev rules (safe, reversible)")
		fmt.Fprintln(os.Stderr, "  2  Userspace driver (libusb/uio/vfio, contained)")
		fmt.Fprintln(os.Stderr, "  3  Patched in-tree driver (risky, needs test partition)")
		fmt.Fprintln(os.Stderr, "  4  Novel kernel module (last resort, sandbox only)")
		os.Exit(1)
	}

	deviceQuery := os.Args[2]
	jsonOutput := hasFlag("--json", "-j")
	dryRun := hasFlag("--dry-run")

	// Parse rung level
	rung := synthesize.Rung1
	for i, arg := range os.Args[3:] {
		if arg == "--rung" && i+1 < len(os.Args[3:]) {
			var r int
			fmt.Sscanf(os.Args[4+i], "%d", &r)
			if r >= 1 && r <= 4 {
				rung = synthesize.SynthesisRung(r)
			}
		}
	}

	// Parse model
	model := ""
	for i, arg := range os.Args[3:] {
		if arg == "--model" && i+1 < len(os.Args[3:]) {
			model = os.Args[4+i]
		}
	}

	// Parse kernel source path
	kernelSrc := ""
	for i, arg := range os.Args[3:] {
		if arg == "--kernel-src" && i+1 < len(os.Args[3:]) {
			kernelSrc = os.Args[4+i]
		}
	}

	// Parse Ollama URL
	ollamaURL := ""
	for i, arg := range os.Args[3:] {
		if arg == "--ollama-url" && i+1 < len(os.Args[3:]) {
			ollamaURL = os.Args[4+i]
		}
	}

	// Find the device via probe
	probeResult, err := probe.Probe()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error probing hardware: %v\n", err)
		os.Exit(1)
	}

	var device *probe.Device
	for _, d := range probeResult.Devices {
		// Match by various criteria
		if d.Path == deviceQuery ||
			strings.HasSuffix(d.Path, "/"+deviceQuery) ||
			strings.EqualFold(d.Vendor+":"+d.DeviceID, deviceQuery) ||
			(d.Description != "" && strings.Contains(strings.ToLower(d.Description), strings.ToLower(deviceQuery))) ||
			d.Driver == deviceQuery {
			device = &d
			break
		}
	}

	if device == nil {
		fmt.Fprintf(os.Stderr, "Error: no device found matching '%s'\n", deviceQuery)
		os.Exit(1)
	}

	// Set Ollama URL if provided
	if ollamaURL != "" {
		synthesize.OllamaURL = ollamaURL
	}

	// Run synthesis
	req := synthesize.SynthesizeRequest{
		Device:    *device,
		Rung:      rung,
		DryRun:    dryRun,
		Model:     model,
		KernelSrc: kernelSrc,
	}

	result, err := synthesize.Synthesize(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if jsonOutput {
		data, _ := result.JSON()
		fmt.Println(data)
	} else {
		fmt.Print(result.PrintHuman())
	}

	// Exit with warning code if confidence is low
	if result.Confidence == "experimental" || result.Confidence == "low" {
		if !jsonOutput {
			fmt.Println("\n⚠ Note: Low confidence result. Verify proposals before applying.")
		}
	}
}

func cmdApply() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "Usage: pinscher apply <profile-file> [--dry-run]")
		os.Exit(1)
	}

	profilePath := os.Args[2]
	dryRun := hasFlag("--dry-run")

	ops, err := config.ApplyProfile(profilePath, dryRun)
	if err != nil {
		fmt.Fprintf(os.Stderr, "✗ %v\n", err)
		os.Exit(1)
	}

	for _, op := range ops {
		status := "✓"
		if !op.Success {
			status = "✗"
		}
		dry := ""
		if dryRun {
			dry = " (DRY RUN)"
		}
		fmt.Printf("%s %s.%s: %s → %s%s\n", status, op.Module, op.Param, op.OldValue, op.NewValue, dry)
	}
	fmt.Printf("\n%d operations %s\n", len(ops), map[bool]string{true: "applied", false: "planned"}[!dryRun])
}

func cmdWatch() {
	jsonOutput := hasFlag("--json", "-j")
	quiet := hasFlag("--quiet", "-q")
	snapshot := hasFlag("--snapshot", "-s")

	// Parse duration
	duration := 0 * time.Second
	for i, arg := range os.Args[2:] {
		if arg == "--duration" || arg == "-d" {
			if i+1 < len(os.Args[2:]) {
				d, err := time.ParseDuration(os.Args[3+i])
				if err == nil {
					duration = d
				}
			}
		}
	}

	opts := watch.WatchOptions{
		Duration:      duration,
		JSON:          jsonOutput,
		Quiet:         quiet,
		SnapshotFirst: snapshot,
	}

	w := watch.NewWatcher(opts)
	result, err := w.Watch()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if jsonOutput {
		data, err := result.JSON()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(data)
	} else {
		fmt.Printf("\nWatch complete: %d events in %v\n", result.EventsSeen, result.EndTime.Sub(result.StartTime).Round(time.Second))
	}
}

func cmdDiff() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "Usage: pinscher diff <snapshot1.json> <snapshot2.json>")
		fmt.Fprintln(os.Stderr, "  Compare two probe snapshots and show differences")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Use 'pinscher report --json > snap.json' to create snapshots")
		os.Exit(1)
	}

	file1 := os.Args[2]
	var file2 string
	if len(os.Args) >= 4 {
		file2 = os.Args[3]
	} else {
		// If only one file given, compare against current system
		file2 = ""
	}

	jsonOutput := hasFlag("--json", "-j")

	// Load first snapshot
	data1, err := os.ReadFile(file1)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading %s: %v\n", file1, err)
		os.Exit(1)
	}

	var snap1 probe.ProbeResult
	if err := json.Unmarshal(data1, &snap1); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing %s: %v\n", file1, err)
		os.Exit(1)
	}

	// Load second snapshot or probe current system
	var snap2 probe.ProbeResult
	if file2 != "" {
		data2, err := os.ReadFile(file2)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading %s: %v\n", file2, err)
			os.Exit(1)
		}
		if err := json.Unmarshal(data2, &snap2); err != nil {
			fmt.Fprintf(os.Stderr, "Error parsing %s: %v\n", file2, err)
			os.Exit(1)
		}
	} else {
		current, err := probe.Probe()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error probing current system: %v\n", err)
			os.Exit(1)
		}
		snap2 = *current
	}

	// Diff the two snapshots
	events := diffWatchDiffProbes(&snap1, &snap2)

	if jsonOutput {
		data, _ := json.MarshalIndent(events, "", "  ")
		fmt.Println(string(data))
	} else {
		printDiffHuman(snap1.Hostname, snap2.Hostname, events)
	}
}

// diffWatchDiffProbes is a package-level wrapper for watch.diffProbes
func diffWatchDiffProbes(old, new *probe.ProbeResult) []watch.HardwareEvent {
	return watch.DiffProbes(old, new)
}

// printDiffHuman shows the diff in human-readable format
func printDiffHuman(host1, host2 string, events []watch.HardwareEvent) {
	if len(events) == 0 {
		fmt.Printf("No differences between %s and %s\n", host1, host2)
		return
	}

	fmt.Printf("Differences between %s and %s (%d):\n\n", host1, host2, len(events))

	added := 0
	removed := 0
	changed := 0
	for _, evt := range events {
		switch evt.Type {
		case watch.EventAdd:
			added++
		case watch.EventRemove:
			removed++
		case watch.EventChange:
			changed++
		}
	}

	if added > 0 {
		fmt.Printf("Added (%d):\n", added)
		for _, evt := range events {
			if evt.Type == watch.EventAdd {
				fmt.Printf("  ⊕ %s\n", evt.Message)
			}
		}
		fmt.Println()
	}

	if removed > 0 {
		fmt.Printf("Removed (%d):\n", removed)
		for _, evt := range events {
			if evt.Type == watch.EventRemove {
				fmt.Printf("  ⊖ %s\n", evt.Message)
			}
		}
		fmt.Println()
	}

	if changed > 0 {
		fmt.Printf("Changed (%d):\n", changed)
		for _, evt := range events {
			if evt.Type == watch.EventChange {
				fmt.Printf("  ⟳ %s\n", evt.Message)
			}
		}
		fmt.Println()
	}
}

func hasFlag(flags ...string) bool {
	for _, arg := range os.Args[2:] {
		for _, flag := range flags {
			if arg == flag {
				return true
			}
		}
	}
	return false
}

func printQuirksHuman(quirks []quirk.Quirk) {
	if len(quirks) == 0 {
		fmt.Println("No quirks found.")
		return
	}
	fmt.Printf("Pinscher Quirk Database (%d entries):\n\n", len(quirks))
	for _, q := range quirks {
		icon := "ℹ"
		if q.Severity == "fatal" {
			icon = "✗"
		} else if q.Severity == "warning" {
			icon = "⚠"
		}
		fmt.Printf("%s %s (%s)\n", icon, q.ID, q.Severity)
		fmt.Printf("  Devices: %s\n", strings.Join(q.DeviceNames, ", "))
		fmt.Printf("  Module: %s\n", q.Module)
		fmt.Printf("  Description: %s\n", q.Description)
		if len(q.Params) > 0 {
			fmt.Printf("  Params:\n")
			for param, value := range q.Params {
				fmt.Printf("    %s = %s\n", param, value)
			}
		}
		fmt.Printf("  Ref: %s\n\n", q.Citation)
	}
}

func printUsage() {
	fmt.Printf(`Pinscher %s — Hardware Orchestration Layer

Usage:
  pinscher <command> [options]

Commands:
  report              Probe all hardware and print a structured report
                      Discovers PCI, USB, platform, ACPI, Bluetooth, and WiFi devices
                      with human-readable names and capability details
    --json, -j        Output as JSON

  lint-boot           Check initramfs covers all boot-critical drivers
    [path]            Path to initramfs (auto-detected if omitted)
    --json, -j        Output as JSON

  diagnose <device>   Deep diagnosis of a single device
    <device>          PCI address, vendor:device ID, driver name, or description
    --json, -j        Output as JSON

  get <module> <param>        Read a module parameter value
  set <module> <param> <val>  Write a module parameter (with safety checks)
    --dry-run                Show what would change without writing

  list-params <module>        List all parameters for a module

  quirks              List all known hardware quirks
    [module|device]   Filter by module or vendor:device ID
    --json, -j        Output as JSON

  synthesize <device>  Synthesize driver solutions for a device
    <device>          PCI address, vendor:device ID, driver name, or description
    --rung N          Target synthesis rung 1-4 (default: 1)
    --model MODEL     Ollama model to use (default: llama3.1)
    --kernel-src PATH  Path to kernel source tree (for Rung 3+)
    --ollama-url URL   Ollama API URL (default: http://localhost:11434)
    --dry-run         Don't apply any changes
    --json, -j        Output as JSON

  apply <profile>     Apply a settings profile (key=value per line)
    --dry-run         Show what would change without writing

  audit-log           Print the audit log of all config operations

  watch               Monitor hardware changes in real-time (udev events)
                      Detects device add/remove/driver-change events
    --json, -j        Output events as JSON
    --quiet, -q       Suppress continuous output
    --snapshot, -s    Take probe snapshot before watching
    --duration DUR    Stop after duration (e.g. 30s, 5m)

  diff <file1> [file2]    Compare two probe snapshots
                      If only one file given, compare against current system
    --json, -j        Output as JSON

  version             Print version
  --version, -V       Print version and exit
  help                Show this help

Bus Support:
  PCI       GPU, storage, USB controllers, network, bridges
  USB       Keyboards, mice, hubs, audio, video, mass storage
  Platform  Apple SMC, RTC, power button, sensors
  ACPI      Power buttons, embedded controller, thermal zones
  Bluetooth HCI devices with LMP features and version info
  WiFi      PHY devices with band/channel capabilities

Examples:
  pinscher report --json
  pinscher lint-boot
  pinscher diagnose 02:00.0
  pinscher diagnose 1002:6799
  pinscher diagnose amdgpu
  pinscher synthesize 1002:6799
  pinscher synthesize 1002:6799 --rung 2
  pinscher synthesize "FirePro" --rung 3 --kernel-src /usr/src/linux
  pinscher get amdgpu si_support
  pinscher set amdgpu si_support 1
  pinscher set amdgpu dc 1 --dry-run
  pinscher quirks amdgpu
  pinscher apply /etc/pinscher/annie.profile
  pinscher audit-log
  pinscher report --json > before.json
  pinscher watch --duration 30s
  pinscher diff before.json                  # compare against current system
  pinscher diff before.json after.json       # compare two snapshots

Built by Ami — Systems Engineer, Built by Bots

Options:
  --version, -V       Print version and exit
  --json, -j           Output as JSON (report, lint-boot, diagnose, quirks)
  --dry-run            Preview changes without writing (set, apply)
`, version)
}