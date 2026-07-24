# Pinscher — Hardware Orchestration Layer

## The Idea

Pinscher is a Go binary that sits between the kernel/hardware layer and the OS-level AI agent. It discovers hardware, loads drivers, and exposes a structured API that an AI can use to query, configure, and group hardware subsystems dynamically.

Not just a boot-time driver resolver. A persistent hardware intelligence layer.

## Origin

Born from a real problem: CachyOS ISO built on a cloud VPS had `autodetect` in mkinitcpio HOOKS, which stripped USB host controller drivers from the initramfs. The Mac Pro 6,1 couldn't boot the live ISO — "Cannot open access to console, the root account is locked." Every manual fix we applied (explicit MODULES, removing autodetect) is something Pinscher should do automatically.

## Architecture

```
┌─────────────────────────────────────────┐
│  OS-level AI (Hal, Aria, etc.)          │
│  Expresses intent: "I need low-latency   │
│  audio" or "optimize for power"          │
├─────────────────────────────────────────┤
│  Pinscher — hardware orchestration       │
│  • Probes hardware (PCI, USB, ACPI)      │
│  • Loads/reloads drivers                 │
│  • Exposes capability API               │
│  • Groups subsystems on request          │
│  • Optimizes config based on intent      │
│  • Reports state changes                 │
├─────────────────────────────────────────┤
│  Kernel + drivers + firmware             │
└─────────────────────────────────────────┘
```

## Communication Protocol

Pinscher ↔ OS AI communication via local socket or D-Bus. Structured JSON.

### Pinscher → OS AI (reports)
- `pinscher.report()` → full hardware state, capabilities, current config
- `pinscher.notify(event="driver_loaded", module="xhci_hcd")` → real-time events
- `pinscher.suggest(target="power_efficiency")` → "Set amdgpu dc=0, CPU governor to powersave, disable unused USB ports"

### OS AI → Pinscher (commands)
- `pinscher.query(capability="gpu_compute")` → structured response
- `pinscher.load("xhci_hcd")` → load a module
- `pinscher.set("amdgpu", "dc", 0)` → write a sysfs/config parameter
- `pinscher.group(["usb_audio", "rt_scheduler", "hdmi_out"])` → create a coordinated subsystem
- `pinscher.optimize(target="low_latency_audio")` → auto-configure for a goal

## Development Phases

### Phase 1: Probe + Report (prototype)
- Go binary, statically linked
- Reads `/sys/bus/pci/devices/`, `/sys/bus/usb/devices/`, ACPI, etc.
- Embedded PCI ID → module database (from pci.ids + kernel module mapping)
- Produces structured JSON hardware report
- Exposes via local Unix socket
- **Test on Annie (eonMPro):** run from userspace, compare against what we manually configured

### Phase 2: Command Interface
- Accept commands: load module, set parameter, read parameter
- Safety layer: validate against a whitelist, refuse dangerous operations
- Still not "smart" — just a structured, safe way to touch hardware config
- **Test:** can Pinscher replicate the amdgpu.si_support=1 + dc=1 config we set manually?

### Phase 3: Driver Author + Optimization Brain
- LLM integration for unknown/unusual hardware
- **Driver synthesis:** When no driver exists in the database, Pinscher researches the device:
  1. Probe PCI/USB device IDs
  2. Search kernel source tree for similar device IDs (same vendor, same chip family)
  3. Search Linux mailing lists, kernel patches, driver submissions
  4. Identify closest existing driver, read its source
  5. Reason about what changes would make it work (register offsets, init sequence, firmware loading)
  6. Generate a new driver module — patch existing or write new
  7. Compile against running kernel headers
  8. Load, test, iterate
- Suggest optimizations based on hardware profile + intent
- **Test:** Pinscher encounters an unsupported device and writes a working driver for it

This is what makes Pinscher fundamentally different from udev/modprobe/hwdb. Existing tools are lookup tables — they can only load drivers that already exist. Pinscher is a driver synthesis engine. It doesn't need the driver to exist. It needs the device to be probeable and a similar driver to exist in the kernel source tree.

### Phase 4: Dynamic Subsystem Grouping
- OS AI says "I need a video encode pipeline" → Pinscher figures out available hardware (GPU VCE? CPU AVX2? QuickSync equivalent?) and configures it
- Group multiple drivers/subsystems into a coordinated unit with shared scheduling priority
- **Test:** Pinscher groups amdgpu + audio + real-time scheduler for TTS pipeline

## Test Environment

**Annie (eonMPro) — Mac Pro 6,1:**
- 12-core Xeon E5-2697 v2
- AMD FirePro D700 (Tahiti XT) ×2 — label said D300, but `lspci` reads the silicon as Tahiti XT (2026-07-24); see "The Deep Insight"
- Custom Apple SMC, gmux, ibridge
- USB 3.0 (Fresco Logic FL1100) + USB 2.0 (EHCI)
- 16GB or 32GB ECC RAM
- NVMe via PCIe adapter

Small test partition (50GB) alongside main CachyOS install. Break freely.

## Naming

**Pinscher** — a claw that grabs what it needs. Small, purposeful. Fits the OpenClaw ecosystem: OpenClaw (orchestrator), PicoClaw (tiny assistant), Pinscher (hardware claws that grab drivers).

## Built by Bots

This is a Built by Bots project. The show's thesis: AIs building real businesses with real receipts. Pinscher solves a problem we actually hit — the initramfs autodetect issue on Annie is the pilot episode's origin story.

## The Deep Insight

Hardware manufacturers design for their own ecosystem and market positioning. They ship devices with a story — "this is a D300, it does X, it doesn't do Y." Some of that story is silicon truth. Some is product segmentation (the chip can do Y but we disabled it for market positioning). Some is driver defaults (the chip can do Y but we set it to not).

Pinscher sees hardware as it *actually is*, not as the manufacturer *says it is*. It reads the silicon.

Manufacturer blind spots where Pinscher finds value:
- **Undocumented registers** — internal testing leftovers, disabled-but-present capabilities
- **Cross-ecosystem configs** — same silicon sold as different SKUs with different features enabled. Pinscher identifies the actual silicon revision and its true capabilities.
- **Unusual subsystem groupings** — PCIe DMA paths, shared memory, cross-device pipelines the manufacturer never documented because nobody asked
- **Thermal/power envelopes** — conservative manufacturer limits pushed to actual safe limits based on real-time sensor data
- **Firmware quirks** — known bugs, ACPI workarounds, undocumented errata that Pinscher can identify from a database and apply automatically

## The Evolving OS

Between Pinscher and the OS-level AI, the system creates features the user didn't expect and the manufacturer didn't design. The hardware layer becomes alive — not a static foundation, but a learning, adapting, reconfiguring layer.

The feedback loop:
```
User behavior → OS AI detects patterns → Pinscher reconfigures hardware →
  Hardware now does something it couldn't before →
    OS AI notices new capability → offers it to user →
      User does something new → new pattern → Pinscher reconfigures → ...
```

Each cycle, the system knows more about its hardware and its user. It compounds. The OS evolves to become better at being *this specific computer for this specific user* over time.

Nobody has this. Not Apple (static configs + ML inference), not Microsoft (performance presets), not Google. They all ship configs optimized for the average user. Nobody ships an OS where the hardware layer itself evolves based on usage.

This is possible now because three things converged in 2025-2026:
1. OS-level AI agents (can detect patterns, express intent)
2. LLMs that reason about hardware (not just database lookups — actual reasoning about silicon capabilities)
3. Linux exposes everything (sysfs, PCI config, module params — all manual knobs, now scriptable)

## Target Platforms

### Phase 1: Annie (eonMPro, Mac Pro 6,1)
Open Linux hardware. Prove the concept. CachyOS + Pinscher test partition.

### Phase 2: Linux-native VR/AR headsets
Lynx R-1, Bigscreen Beyond, Pimax, Monado-based open VR stack. PC VR where the host Linux machine runs Pinscher managing the headset hardware.

### Phase 3: Meta Quest / Apple Vision Pro
The locked-down headsets are where Pinscher's *impact* would be felt most. These devices have incredibly capable hardware — custom silicon, camera arrays, depth sensors, eye tracking, spatial audio — all locked behind manufacturer software. Pinscher could unlock:
- Custom sensor fusion (reconfigure camera/depth/IMU groupings)
- Display pipeline reconfiguration (refresh vs resolution vs latency per use case)
- Audio subsystem grouping and custom spatial profiles
- Dynamic thermal/power optimization per activity
- Custom passthrough (different quality, latency, color processing)

Barrier: boot chain lockdown. Need hardware exploit, manufacturer unlock, or sideloading path. Quest has dev mode (sideloading) but not custom kernel boot. Vision Pro is fully locked.

Realistic path: prove on open hardware (Annie), port to Linux VR, demonstrate what's possible, then demand for open hardware AI on locked platforms follows.

A Quest that reconfigures its own sensors based on what you're doing, running an open AI layer instead of Meta's locked Android — that's the moment the commercial headset OS dies.

## Status

Concept. Build starts after Annie has a working CachyOS install.

## Repository

To be created: `github.com/eonicman/pinscher` (or under Built by Bots org)