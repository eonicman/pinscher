# Pinscher Changelog

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