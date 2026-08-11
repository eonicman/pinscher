package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/eonicman/pinscher/internal/config"
	"github.com/eonicman/pinscher/internal/diagnose"
	"github.com/eonicman/pinscher/internal/lintboot"
	"github.com/eonicman/pinscher/internal/probe"
	"github.com/eonicman/pinscher/internal/quirk"
)

const version = "0.4.0"

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
	case "apply":
		cmdApply()
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

  apply <profile>     Apply a settings profile (key=value per line)
    --dry-run         Show what would change without writing

  audit-log           Print the audit log of all config operations

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
  pinscher get amdgpu si_support
  pinscher set amdgpu si_support 1
  pinscher set amdgpu dc 1 --dry-run
  pinscher quirks amdgpu
  pinscher apply /etc/pinscher/annie.profile
  pinscher audit-log

Built by Ami — Systems Engineer, Built by Bots

Options:
  --version, -V       Print version and exit
  --json, -j           Output as JSON (report, lint-boot, diagnose, quirks)
  --dry-run            Preview changes without writing (set, apply)
`, version)
}