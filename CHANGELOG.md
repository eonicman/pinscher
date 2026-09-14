# Pinscher Changelog

## v0.9.0 — Serve Daemon (Persistent Hardware Intelligence Layer)

### New: `pinscher serve`

The CONCEPT.md Phase 1 spec — "exposes via local Unix socket" — is now real.
`pinscher serve` runs a long-lived daemon exposing the hardware API over a
local Unix socket using newline-delimited JSON, so AI agents and scripts can
query live hardware state without a cold process start per query.

- **Read-only methods:** `ping`, `version`, `report`, `lint-boot`, `diagnose`,
  `quirks`, `profile`, `profiles`, `recommend`, `list-params`, `get`, `audit-log`
- **Mutating method:** `set` — gated behind `--allow-write`; respects the same
  safety ladder as the CLI (unknown params blocked, audit log always written)
- **Socket hygiene:** created with 0600 permissions; stale socket files from
  unclean shutdowns are reclaimed automatically; a second live instance on
  the same socket is refused
- **Protocol details:** request IDs echoed verbatim (any JSON value); numeric
  params keep exact source text (`120000` never decays to `1.2e+05`, thanks to
  `json.Number` decoding); panics are converted to error responses so one bad
  request can't kill the daemon; unknown-method errors advertise the supported
  method list so agent clients can self-correct
- **Flags:** `--socket PATH`, `--allow-write`, `--duration DUR` (auto-stop,
  for tests), `--quiet`
- Responses are byte-identical to the equivalent `--json` CLI output
  (verified: `lint-boot` over socket == CLI JSON on real hardware)

### Fixed

- `pinscher --version` reported 0.7.0 after v0.8.0 shipped — version const now
  correctly bumped per release
- README status line was stale (said v0.6.0)

### Added

- Backfilled CHANGELOG entries for v0.7.0 and v0.8.0 (were committed without
  changelog sections)
- 19 unit tests for the serve package: protocol framing, method dispatch,
  safety gating, numeric fidelity, stale-socket recovery, concurrent clients,
  multi-request connections

---

## v0.8.0 — quirk-submit, profile recommend in boot service, usage docs

### New: `pinscher quirk-submit`

Generate a GitHub issue template for reporting a new hardware quirk:

- Includes device info, existing quirks, and a proposed quirk template
- `--probe` flag embeds full hardware probe data for reproduction
- `--json` output for programmatic use

### Other

- Updated systemd boot-check service (`dist/pinscher-boot-check.service`) to
  run profile recommendations after lint-boot + report
- Full usage docs for watch, diff, profile, and quirk-submit; examples section
  covers all command variants

---

## v0.7.0 — Profile System, Hardware Presets, Profile Recommendation

### New: `pinscher profile`

List, show, apply, and recommend hardware profiles:

- **6 built-in profiles:** gaming, low-power, audio-production, vr, server,
  macpro61
- **Profile enrichment:** merge quirk database entries into profile settings
  (`--enrich`)
- **Profile recommendation:** auto-suggest profiles based on detected hardware
  (`pinscher profile recommend`)
- **Hardware match criteria:** driver, vendor_id, device_id, class, bus
- **Profile conflicts:** gaming conflicts with low-power, etc.
- **Safety model:** blocked params skipped with explanation
- **JSON output** for all profile commands; **dry-run** support for apply

Profile system supports built-in named profiles, custom JSON profile files,
legacy key=value profile files (backward compat), quirk enrichment, and
auto-recommendation.

---

## v0.6.0 — Watch Mode, Diff, Expanded Quirk Database

### New: `pinscher watch` command

Real-time hardware monitoring via udev events. Detects device add/remove/change
as they happen. Falls back to polling if udevadm is unavailable.

- `--json` for scripted consumption
- `--quiet` to suppress continuous output
- `--snapshot` to take a before/after probe snapshot
- `--duration 30s` to stop after a time period
- Events include device bus, vendor/device IDs, driver, and description

### New: `pinscher diff` command

Compare two hardware probe snapshots to find differences. Shows added, removed,
and changed devices (e.g., driver changes).

```bash
pinscher report --json > before.json
# make hardware changes
pinscher diff before.json              # compare against current system
pinscher diff before.json after.json    # compare two snapshots
```

### Expanded Quirk Database

22 entries across 9 categories (was 7 entries, 3 categories):

- **GPU**: AMD SI support, DC, reset method; NVIDIA DPM
- **USB**: FL1100, ASMedia ASM1142
- **Storage**: NVMe APST, Samsung 970 EVO+ firmware
- **Audio**: Intel HDA model fixup, Realtek ALC892/897, Framework audio codec
- **Network**: Realtek RTL8125 ASPM, Intel AX200/210 WiFi power save
- **Power**: Framework S3 sleep, RPi4 USB-C
- **Input**: ThinkPad TrackPoint, MacBook Pro TouchBar
- **Thunderbolt**: ThinkPad T480s/T490, Dell XPS 13
- **Apple**: SMC, gmux (existing, Mac Pro 6,1)

### Other
- Updated README with new commands and expanded quirk table
- Added CONTRIBUTING.md
- Tests updated for expanded quirk database
- All 9 categories now represented in quirk database

---

## v0.5.0 — Driver Synthesis Engine

### New: `pinscher synthesize` command

The flagship feature: Pinscher can now propose driver solutions for devices without
drivers, using a safety ladder from safe to spicy:

- **Rung 1** (safe): Quirk/param/udev rule generation from embedded database + LLM reasoning
- **Rung 2** (contained): Userspace driver proposals (libusb for USB, VFIO for PCI)
- **Rung 3** (risky): Kernel source analysis — find similar drivers in the source tree
- **Rung 4** (last resort): Novel kernel module synthesis (sandbox only)

### How it works

1. **Database lookup**: Checks embedded quirk database for known device issues
   (amdgpu Tahiti XT quirks, etc.) — instant, verified, high confidence
2. **Module resolution**: If a device has a modalias but no loaded driver, checks
   `modprobe --resolve-alias` and proposes loading the correct module
3. **LLM synthesis**: Queries Ollama (local LLM) with full device context for
   proposals at the target rung level
4. **Userspace driver proposals**: For USB/PCI devices, proposes libusb or VFIO
   passthrough as safer alternatives to kernel modules
5. **Kernel source search**: At Rung 3+, searches the kernel source tree for
   vendor/device ID matches in existing drivers

### Output

Each proposal includes:
- Type (quirk, module_param, udev_rule, userspace_driver, kernel_patch)
- Safety rung (1-4)
- Action description and shell commands
- Config files to write (e.g., `/etc/modprobe.d/pinscher-amdgpu.conf`)
- Rollback commands
- Reasoning and citation URLs
- Verification status

### Usage

```bash
# Safe: database + modprobe + LLM (Rung 1)
pinscher synthesize 1002:6799

# Userspace driver proposals (Rung 2)
pinscher synthesize 1002:6799 --rung 2

# With kernel source analysis (Rung 3)
pinscher synthesize 1002:6799 --rung 3 --kernel-src /usr/src/linux

# Skip LLM (database + modprobe only, instant)
pinscher synthesize 1002:6799 --model none

# JSON output
pinscher synthesize 1002:6799 --json

# Dry run (no changes applied)
pinscher synthesize 1002:6799 --dry-run
```

### Models

- Default LLM: `qwen3:8b` (local via Ollama)
- Use `--model none` to skip LLM and get database-only results
- Use `--model phi3:mini` for faster but less accurate results
- Use `--ollama-url` for custom Ollama endpoints

---

## v0.4.0 — Multi-Bus Probing

### New bus probes
- Platform: OF compatible strings, modalias, applesmc/macsmc devices
- ACPI: HID/CID/UID/status extraction, filtered 316 `device:*` containers
- Bluetooth: Full HCI/LMP capability extraction, resolves vendor/product from USB parent
- WiFi: PCI parent resolution, HT/VHT flags, `iw phy# info` details

### Other
- PCI/USB class name lookups (40+ PCI classes, 20+ USB classes)
- Device.Capabilities map for bus-specific key-value pairs
- lint-boot: platform/ACPI boot-critical checks
- Bug fixes: Bluetooth vendor resolution, ACPI filtering
- Tests: probe_newbus_test.go — all passing

---

## v0.3.0 — Diagnose, Quirk Database, Config

### New commands
- `pinscher diagnose <device>` — deep diagnosis of a single device
- `pinscher quirks [module|device]` — list known hardware quirks
- `pinscher set <module> <param> <value>` — write module parameters
- `pinscher get <module> <param>` — read module parameters
- `pinscher list-params <module>` — list all parameters for a module
- `pinscher apply <profile>` — apply a settings profile
- `pinscher audit-log` — print audit log of config operations

### Quirk database
- Embedded quirk database with provenance (citations, kernel version ranges)
- AMD FirePro D700 / Tahiti XT quirks (si_support=1, dc=1)
- Fresco Logic FL1100 USB 3.0 firmware timing issue
- Apple SMC and GMUX documentation entries

### Diagnose engine
- Modalias resolution (modprobe --resolve-alias)
- Blacklist detection (finds which file blacklists a module)
- Firmware missing detection
- Quirk applicability checking (verifies params are set correctly)
- dmesg error scanning for device-specific issues

---

## v0.2.0 — PCI ID Database, Packaging

### New
- Embedded PCI ID database (pci.ids) for human-readable device names
- Vendor/device ID normalization and lookup
- PKGBUILD for Arch Linux packaging
- systemd unit for pinscher daemon (future)
- mkinitcpio hook for lint-boot integration

---

## v0.1.0 — Initial Release

### New
- `pinscher report` — probe all hardware (PCI + USB)
- `pinscher lint-boot` — check initramfs covers all boot-critical drivers
- Basic sysfs walking for PCI and USB buses
- Human-readable and JSON output
- Boot-critical device detection (storage, USB host, keyboard, GPU)