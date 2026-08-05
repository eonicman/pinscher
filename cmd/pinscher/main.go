package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/eonicman/pinscher/internal/lintboot"
	"github.com/eonicman/pinscher/internal/probe"
)

const version = "0.1.0"

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "report":
		cmdReport()
	case "lint-boot":
		cmdLintBoot()
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

	// Check for --json flag
	jsonOutput := false
	for _, arg := range os.Args[2:] {
		if arg == "--json" || arg == "-j" {
			jsonOutput = true
		}
	}

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
			// Will handle after getting result
		} else if !strings.HasPrefix(arg, "-") {
			initramfsPath = arg
		}
	}

	result, err := lintboot.LintBoot(initramfsPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Check for --json flag
	jsonOutput := false
	for _, arg := range os.Args[2:] {
		if arg == "--json" || arg == "-j" {
			jsonOutput = true
		}
	}

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
			os.Exit(1) // Exit non-zero if issues found (useful for CI/hooks)
		}
	}
}

func printReportHuman(r *probe.ProbeResult) {
	fmt.Printf("Pinscher Hardware Report — %s\n", r.Hostname)
	fmt.Printf("Kernel: %s\n\n", r.Kernel)
	fmt.Printf("Devices: %d total (%d with driver, %d without)\n\n",
		r.Summary.Total, r.Summary.WithDriver, r.Summary.NoDriver)

	// Group by bus
	for _, dev := range r.Devices {
		icon := "✓"
		if dev.Driver == "" {
			icon = "✗"
		}
		desc := dev.Description
		if desc == "" {
			desc = dev.Vendor + ":" + dev.DeviceID
		}
		fmt.Printf("  %s [%s] %s", icon, dev.Bus, desc)
		if dev.Driver != "" {
			fmt.Printf(" (%s)", dev.Driver)
		} else {
			fmt.Printf(" — NO DRIVER")
		}
		fmt.Println()
	}
}

func printUsage() {
	fmt.Printf(`Pinscher %s — Hardware Orchestration Layer

Usage:
  pinscher <command> [options]

Commands:
  report          Probe all hardware and print a structured report
    --json, -j    Output as JSON

  lint-boot       Check initramfs covers all boot-critical drivers
    [path]        Path to initramfs (auto-detected if omitted)
    --json, -j    Output as JSON

  version         Print version

  help            Show this help

Examples:
  pinscher report --json
  pinscher lint-boot
  pinscher lint-boot /boot/initramfs-linux.img --json

Built by Ami — Systems Engineer, Built by Bots
`, version)
}