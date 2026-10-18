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
	"github.com/eonicman/pinscher/internal/profile"
	"github.com/eonicman/pinscher/internal/quirk"
	"github.com/eonicman/pinscher/internal/serve"
	"github.com/eonicman/pinscher/internal/submit"
	"github.com/eonicman/pinscher/internal/synthesize"
	"github.com/eonicman/pinscher/internal/watch"
)

const version = "0.9.3"

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
	case "profile":
		cmdProfile()
	case "quirk-submit":
		cmdQuirkSubmit()
	case "serve":
		cmdServe()
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

	// Opt-in: a saved report is also used for local snapshots and diffs, so the
	// default output is unchanged. Use --anonymize before posting one publicly.
	if hasFlag("--anonymize") {
		result.Anonymize()
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
	args := os.Args[2:]
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--assert" {
			// Ghost, 2026-10-18: skip the flag's value. Before this, the
			// value of `--assert x=y` was taken as the initramfs path
			// (found on real hardware; no initramfs on the dev VM).
			i++
		} else if arg == "--json" || arg == "-j" {
		} else if !strings.HasPrefix(arg, "-") {
			initramfsPath = arg
		}
	}

	result, err := lintboot.LintBoot(initramfsPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// --assert (Ghost, 2026-10-17): checked separately from LintBoot() itself
	// so LintBoot's signature/behavior stays untouched for existing callers
	// (notably internal/serve, which dispatches straight to it). Merged into
	// the same LintResult for a unified report + exit code.
	if asserts := flagValues("--assert"); len(asserts) > 0 {
		assertResults, err := lintboot.CheckAssertions(asserts)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		result.Assertions = assertResults
		for _, a := range assertResults {
			if !a.Pass {
				result.AllClear = false
			}
		}
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

	// --assert failures always exit 1, in BOTH output modes (Ghost,
	// 2026-10-17). Deliberately separate from the pre-existing Issues/
	// AllClear exit path above, which I'm leaving untouched: --json mode
	// never exits nonzero even when boot-critical Issues are found, only
	// the human branch does. That looks like the same class of bug as
	// diff's exit-0-always gap found earlier tonight — EXCEPT lint-boot has
	// a real shipped consumer, dist/pinscher-boot-check.service, a 3-step
	// oneshot (lint-boot --json / report --json / profile recommend --json)
	// that currently relies on none of them failing the unit. Changing that
	// silently could stop later steps from running and flip the unit to
	// "failed" on every boot with so much as an info-level finding. Needs a
	// deliberate decision by the maintainers, not a quiet fix bundled in here — so
	// this PR only makes --assert (brand new, zero existing dependents)
	// honest, and leaves the older gap as a documented, open finding.
	if asserts := flagValues("--assert"); len(asserts) > 0 {
		for _, a := range result.Assertions {
			if !a.Pass {
				os.Exit(1)
			}
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

	// --fail-on-change (Ghost, 2026-10-17): diff previously always exited 0,
	// even when it found real differences, so it couldn't drive a shell
	// &&/|| regression gate — found while writing the fleet/enterprise docs
	// (kvm-passthrough-validation.md's update-day regression check assumed
	// a --fail-on flag could do this; the real gap was the exit code, not
	// the flag name). Opt-in rather than changing the default, so any
	// existing script relying on diff always exiting 0 isn't surprised by
	// this release. Making it the default is an open question.
	if hasFlag("--fail-on-change") && len(events) > 0 {
		os.Exit(1)
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

func cmdQuirkSubmit() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "Usage: pinscher quirk-submit <device> [--probe]")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Generate a GitHub issue template for submitting a new hardware quirk.")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "  <device>    PCI address, vendor:device ID, driver name, or description")
		fmt.Fprintln(os.Stderr, "  --probe     Include full hardware probe data in the issue")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "The generated issue is anonymized (hostname replaced, MAC addresses masked,")
		fmt.Fprintln(os.Stderr, "serial numbers redacted) because it is meant to be posted publicly.")
		os.Exit(1)
	}

	deviceQuery := os.Args[2]
	includeProbe := hasFlag("--probe", "-p")

	title, body, err := submit.GenerateIssue(deviceQuery, includeProbe)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	jsonOutput := hasFlag("--json", "-j")
	if jsonOutput {
		data := map[string]string{
			"title": title,
			"body":  body,
		}
		out, _ := json.MarshalIndent(data, "", "  ")
		fmt.Println(string(out))
	} else {
		fmt.Printf("Title: %s\n\n%s", title, body)
	}
}

func cmdProfile() {
	jsonOutput := hasFlag("--json", "-j")
	dryRun := hasFlag("--dry-run")
	enrich := hasFlag("--enrich", "-e")

	if len(os.Args) < 3 {
		// List available profiles
		names := profile.ListProfiles()
		if jsonOutput {
			data, _ := json.MarshalIndent(names, "", "  ")
			fmt.Println(string(data))
		} else {
			fmt.Println("Available profiles:")
			for _, name := range names {
				p, _ := profile.LoadProfile(name)
				if p != nil {
					fmt.Printf("  %-20s %s\n", name, p.Description)
				}
			}
			fmt.Println()
			fmt.Println("Usage:")
			fmt.Println("  pinscher profile <name>           Show profile details")
			fmt.Println("  pinscher profile <name> --apply     Apply profile settings")
			fmt.Println("  pinscher profile <name> --enrich    Enrich with hardware-specific quirks")
			fmt.Println("  pinscher profile recommend           Recommend profiles for current hardware")
			fmt.Println("  pinscher profile <file.json>         Load profile from file")
		}
		return
	}

	subcmd := os.Args[2]

	// Handle subcommands
	switch subcmd {
	case "recommend", "rec":
		cmdProfileRecommend(jsonOutput, dryRun, enrich)
		return
	}

	// Load the profile
	p, err := profile.LoadProfile(subcmd)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Enrich with hardware quirks if requested
	if enrich {
		probeResult, err := probe.Probe()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: probe failed, skipping enrichment: %v\n", err)
		} else {
			p = profile.EnrichProfileFromQuirks(p, probeResult)
		}
	}

	// Check for --apply flag
	apply := hasFlag("--apply", "-a")
	if apply {
		result, err := profile.ApplyProfile(p, dryRun)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		if jsonOutput {
			data, _ := json.MarshalIndent(result, "", "  ")
			fmt.Println(string(data))
		} else {
			printProfileApplyResult(result, p, dryRun)
		}
	} else {
		// Just show the profile
		if jsonOutput {
			data, err := p.JSON()
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			fmt.Println(data)
		} else {
			fmt.Print(profile.PrintProfileHuman(p))
		}
	}
}

func cmdProfileRecommend(jsonOutput, dryRun, enrich bool) {
	probeResult, err := probe.Probe()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: probe failed: %v\n", err)
		os.Exit(1)
	}

	recommended := profile.RecommendProfiles(probeResult)

	if jsonOutput {
		var profiles []string
		for _, p := range recommended {
			profiles = append(profiles, p.Name)
		}
		data, _ := json.MarshalIndent(profiles, "", "  ")
		fmt.Println(string(data))
	} else {
		if len(recommended) == 0 {
			fmt.Println("No profiles recommended for your hardware.")
			return
		}

		fmt.Printf("Recommended profiles for %s:\n\n", probeResult.Hostname)
		for _, p := range recommended {
			fmt.Printf("  %-20s %s\n", p.Name, p.Description)
			fmt.Printf("  %-20s %d settings\n\n", "", len(p.Settings))
		}

		fmt.Println("Apply with: pinscher profile <name> --apply")
		fmt.Println("Preview with: pinscher profile <name> --apply --dry-run")
	}
}

func printProfileApplyResult(result *profile.ApplyResult, p *profile.Profile, dryRun bool) {
	dry := ""
	if dryRun {
		dry = " (DRY RUN)"
	}
	fmt.Printf("Profile: %s%s\n\n", p.Name, dry)

	if len(result.Applied) > 0 {
		fmt.Printf("Applied (%d):\n", len(result.Applied))
		for _, op := range result.Applied {
			status := "✓"
			if !op.Success {
				status = "✗"
			}
			fmt.Printf("  %s %s.%s: %s → %s\n", status, op.Module, op.Param, op.OldValue, op.NewValue)
			if op.Error != "" {
				fmt.Printf("    Note: %s\n", op.Error)
			}
		}
		fmt.Println()
	}

	if len(result.Skipped) > 0 {
		fmt.Printf("Skipped (%d):\n", len(result.Skipped))
		for _, s := range result.Skipped {
			fmt.Printf("  ⊘ %s.%s = %s\n", s.Setting.Module, s.Setting.Param, s.Setting.Value)
			fmt.Printf("    Reason: %s\n", s.Reason)
		}
		fmt.Println()
	}

	if len(result.Errors) > 0 {
		fmt.Printf("Errors (%d):\n", len(result.Errors))
		for _, e := range result.Errors {
			fmt.Printf("  ✗ %s\n", e)
		}
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

// flagValues collects every value given to a repeatable flag, e.g.
// `--assert a=1 --assert b=2` -> ["a=1", "b=2"]. Added by Ghost, 2026-10-17,
// for lint-boot --assert — the first repeatable flag in this CLI, so there's
// no existing helper for it; hasFlag only detects presence.
func flagValues(name string) []string {
	var values []string
	args := os.Args[2:]
	for i, arg := range args {
		if arg == name && i+1 < len(args) {
			values = append(values, args[i+1])
		}
	}
	return values
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

func cmdServe() {
	socketPath := ""
	allowWrite := false
	quiet := false
	duration := time.Duration(0)

	args := os.Args[2:]
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--allow-write":
			allowWrite = true
		case arg == "--quiet" || arg == "-q":
			quiet = true
		case arg == "--socket":
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "Error: --socket requires a path")
				os.Exit(1)
			}
			i++
			socketPath = args[i]
		case strings.HasPrefix(arg, "--socket="):
			socketPath = strings.TrimPrefix(arg, "--socket=")
		case arg == "--duration":
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "Error: --duration requires a value (e.g. 30s)")
				os.Exit(1)
			}
			i++
			arg = args[i]
			fallthrough
		case strings.HasPrefix(arg, "--duration="):
			val := strings.TrimPrefix(arg, "--duration=")
			d, err := time.ParseDuration(val)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: invalid --duration %q: %v\n", val, err)
				os.Exit(1)
			}
			duration = d
		default:
			fmt.Fprintf(os.Stderr, "Error: unknown flag %q (serve supports --socket PATH, --allow-write, --duration DUR, --quiet)\n", arg)
			os.Exit(1)
		}
	}

	serve.Version = version
	if err := serve.Run(socketPath, allowWrite, duration, quiet); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
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
    --anonymize       Replace the hostname, mask MAC addresses (keeps the vendor
                      prefix) and redact serial numbers. Use before posting a
                      report publicly.

  lint-boot          Check initramfs covers all boot-critical drivers
    [path]            Path to initramfs (auto-detected if omitted)
    --json, -j        Output as JSON
    --assert field:device=value   Assert a device's field equals value
                      (repeatable). Fields: driver, bus, vendor, device_id,
                      class, description. Fails (exit 1, both output modes)
                      if any assertion doesn't hold — independent of the
                      initramfs check above.

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

  quirk-submit <device>  Generate a GitHub issue template for a new quirk
    <device>          PCI address, vendor:device ID, driver name, or description
    --probe, -p       Include full hardware probe data
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

  profile             List available hardware profiles
    [name]            Show profile details
    --apply, -a       Apply the profile
    --dry-run         Preview changes without writing
    --enrich, -e      Enrich profile with hardware-specific quirks
    --json, -j        Output as JSON
    recommend          Recommend profiles for current hardware

  serve               Run as a persistent daemon — the hardware
                      intelligence layer. Exposes a JSON API over a local
                      Unix socket (newline-delimited JSON requests/responses).
                      Read-only methods: ping, version, report, lint-boot,
                      diagnose, quirks, profile, profiles, recommend,
                      list-params, get, audit-log. Mutating method: set
                      (only with --allow-write; socket is 0600).
    --socket PATH     Socket path (default: /run/pinscher.sock as root,
                      else $XDG_RUNTIME_DIR/pinscher.sock)
    --allow-write     Enable the mutating "set" method
    --duration DUR    Auto-stop after duration (e.g. 30s) — useful for tests
    --quiet, -q       Don't print the listening banner

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
    --fail-on-change  Exit 1 if any differences are found (default: always exits 0)

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
  pinscher lint-boot --assert "driver:0000:02:00.0=vfio-pci"
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
  pinscher diff before.json after.json --fail-on-change || echo "something changed"
  pinscher profile recommend
  pinscher profile gaming --apply --dry-run
  pinscher profile macpro61 --enrich --apply
  pinscher quirk-submit amdgpu              # generate GitHub issue template
  pinscher quirk-submit 1002:6799 --probe   # with full probe data
  pinscher serve                            # persistent daemon (read-only)
  pinscher serve --socket=/tmp/p.sock -q    # custom socket, quiet
  pinscher serve --allow-write              # enable "set" mutations
  echo '{"id":1,"method":"report"}' | \
    socat - UNIX-CONNECT:/run/pinscher.sock # one-shot socket query

Built by Ami — Systems Engineer, Built by Bots

Options:
  --version, -V       Print version and exit
  --json, -j           Output as JSON (report, lint-boot, diagnose, quirks)
  --dry-run            Preview changes without writing (set, apply)
`, version)
}
