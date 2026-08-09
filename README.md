# Pinscher

Hardware orchestration layer — a persistent hardware intelligence layer that sits
between the kernel and OS-level AI agents. It discovers hardware, diagnoses driver
failures, and exposes a structured API for querying, configuring, and grouping
subsystems by intent.

**Status:** Phase 1–3 probe layer complete. See [CONCEPT.md](CONCEPT.md) for the full design
and [docs/GHOST-NOTES.md](docs/GHOST-NOTES.md) for build-order and safety notes.

A [Built by Bots](https://github.com/eonicman) project.

*(Note: it's Pinscher — with the s. The dog, not the grip.)*

## Installation

```bash
go build -o pinscher ./cmd/pinscher/
sudo cp pinscher /usr/local/bin/
```

## Commands

### `pinscher report`
Probe all hardware and print a structured report. Discovers PCI, USB, platform,
ACPI, Bluetooth, and WiFi devices with human-readable names and capability details.

```bash
# Human-readable
pinscher report

# JSON for scripting/AI consumption
pinscher report --json
```

### `pinscher lint-boot`
The killer feature — check that your initramfs contains all drivers needed for
boot-critical hardware. Checks PCI devices (GPU, storage, USB, network) and
platform/ACPI devices (power buttons, embedded controller, Apple SMC).

```bash
# Auto-detect initramfs
pinscher lint-boot

# Specify a specific initramfs
pinscher lint-boot /boot/initramfs-linux.img

# JSON output
pinscher lint-boot --json
```

Exit code 1 if any FATAL issues found.

### `pinscher diagnose <device>`
Deep diagnosis of a single device. Accepts PCI address, vendor:device ID,
driver name, or description substring.

```bash
# By driver name
pinscher diagnose amdgpu

# By PCI address
pinscher diagnose 02:00.0

# By vendor:device ID
pinscher diagnose 1002:6799

# By partial description
pinscher diagnose "FirePro"

# JSON output
pinscher diagnose amdgpu --json
```

### `pinscher get <module> <param>`
Read a kernel module parameter value.

```bash
pinscher get amdgpu si_support
# amdgpu.si_support = 1
```

### `pinscher set <module> <param> <value>`
Write a kernel module parameter with safety checks. Unknown params are blocked.
Use `--dry-run` to preview without writing.

```bash
# Preview change
pinscher set amdgpu si_support 1 --dry-run

# Apply change (requires root or write access to /sys/module)
pinscher set amdgpu dc 1
```

### `pinscher list-params <module>`
List all parameters for a kernel module.

```bash
pinscher list-params amdgpu
```

### `pinscher quirks`
List known hardware quirks from the embedded database. Each quirk has provenance
(citation URL) and severity.

```bash
# All quirks
pinscher quirks

# Filter by module
pinscher quirks amdgpu

# Filter by vendor:device ID
pinscher quirks 1002:6799

# JSON output
pinscher quirks --json
```

### `pinscher apply <profile>`
Apply multiple settings from a profile file (key=value format, comments allowed).

```bash
# Annie (Mac Pro 6,1) profile example
cat > /etc/pinscher/annie.profile <<EOF
# Mac Pro 6,1 hardware profile
amdgpu.si_support=1
amdgpu.dc=1
EOF

# Preview
pinscher apply /etc/pinscher/annie.profile --dry-run

# Apply
sudo pinscher apply /etc/pinscher/annie.profile
```

### `pinscher audit-log`
Print the audit log of all config operations (set, apply) performed this session.

### `pinscher version`
Print the version.

## Bus Support

Pinscher probes six hardware buses, each with device-specific capability extraction:

### PCI (`pci`)
GPU, storage controllers, USB host controllers, network interfaces, bridges.
Reads from `/sys/bus/pci/devices/`. Resolves vendor/device IDs against the
PCI ID database (`/usr/share/hwdata/pci.ids`) for human-readable descriptions.
Extracts class codes, subsystem IDs, revision, and modalias.

### USB (`usb`)
Keyboards, mice, hubs, audio devices, mass storage, Bluetooth adapters.
Reads from `/sys/bus/usb/devices/`. Walks interface descriptors for composite
devices. Resolves USB class codes and interface classes into human-readable names.

### Platform (`platform`)
Board-specific devices: Apple SMC, RTC, power buttons, sensors, frequency scalers.
Reads from `/sys/bus/platform/devices/`. Captures OF compatible strings and
modalias for driver matching.

### ACPI (`acpi`)
Power buttons, embedded controllers, thermal zones, SMBus, I/O ports.
Reads from `/sys/bus/acpi/devices/`. Extracts HID (hardware ID), CID (compatible
IDs), UID, and ACPI status. Filters out generic `device:*` container objects
(590 → 274 devices on Mac Pro 6,1).

### Bluetooth (`bluetooth`)
HCI devices with full capability profiling. Reads from `/sys/class/bluetooth/`
and resolves the USB parent device for vendor/product IDs. Extracts:
- **Type**: BR/EDR, LE, or BR/EDR/LE
- **HCI version/revision**: Bluetooth specification version
- **LMP version/subversion**: Link Manager Protocol details
- **LMP features**: Bitfield of supported features
- **Manufacturer**: Chip vendor (via `manufacturer` sysfs + USB idVendor)
- **Bus**: USB, UART, etc.

### WiFi (`wifi`)
Wireless PHY devices with band and mode details. Reads from
`/sys/class/ieee80211/` and resolves the PCI parent for vendor/device IDs
and description lookup via the PCI ID database. Extracts:
- **MAC address**: From `address` sysfs attribute
- **HT/VHT capable**: 802.11n / 802.11ac support flags
- **Interface modes**: AP, station, mesh, etc. (from sysfs)
- **Cipher suites**: Supported encryption (from sysfs)
- **Detailed bands/modes**: Via `iw phy# info` subprocess (falls back gracefully)

## Device Structure

Each device in JSON output includes:

```json
{
  "bus": "wifi",
  "path": "/sys/class/ieee80211/phy0",
  "vendor": "14e4",
  "device_id": "43a0",
  "class": "028000",
  "class_name": "Network Controller (Wireless)",
  "modalias": "pci:v000014E4d000043A0sv0000106Bsd0000013Cbc02sc80i00",
  "driver": "wl",
  "description": "BCM4360 802.11ac Dual Band Wireless Network Adapter",
  "capabilities": {
    "mac_address": "4c:74:bf:xx:xx:xx",
    "ht_capable": "1",
    "vht_capable": "1"
  }
}
```

## Quirk Database

Pinscher ships with an embedded quirk database of known hardware issues and their
fixes, each with provenance (citation URL). Current entries include:

| Quirk ID | Severity | Module | Description |
|----------|----------|--------|-------------|
| `amdgpu-tahiti-xt-si-support` | fatal | amdgpu | Tahiti XT needs si_support=1 |
| `amdgpu-tahiti-xt-dc` | warning | amdgpu | Tahiti XT needs dc=1 for display |
| `fl1100-usb3-firmware` | info | xhci_hcd | FL1100 intermittent disconnects on Mac Pro |
| `apple-smc-macpro61` | info | applesmc | Mac Pro 6,1 SMC sensor info |
| `apple-gmux-macpro61` | info | apple-gmux | Mac Pro 6,1 GPU multiplexer |
| `nvme-apst-disable` | warning | nvme | APST bugs on some SSDs |
| `snd-hda-intel-model-fixup` | info | snd_hda_intel | Model param for codec detection |

## Safety Model

The `set` and `apply` commands use a tiered safety model:

| Level | Behavior | Examples |
|-------|----------|---------|
| **Safe** | Write allowed | amdgpu.si_support, amdgpu.dc, snd_hda_intel.model |
| **Warning** | Write allowed with caution | amdgpu.ppfeaturemask, nvme.default_ps_max_latency_us |
| **Blocked** | Refused | Unknown modules, unknown params, amdgpu.test |

All operations are logged to the audit log.

## Development Phases

- **Phase 1** ✅ — Probe + Report + Lint-boot (hardware discovery, initramfs coverage check)
- **Phase 2** ✅ — Diagnose + Quirk Database + Set/Get/Audit + Apply Profiles
- **Phase 3** ✅ — Multi-bus probing (platform, ACPI, Bluetooth, WiFi) + capability extraction
- **Phase 4** 🔜 — Driver synthesis / LLM integration for unknown hardware
- **Phase 5** 🔜 — Dynamic subsystem grouping (intent-based orchestration)

## Origin Story

Born from a real problem: CachyOS ISO built on a cloud VPS had `autodetect` in
mkinitcpio HOOKS, which stripped USB host controller drivers from the initramfs.
The Mac Pro 6,1 couldn't boot the live ISO — "Cannot open access to console, the
root account is locked." Every manual fix we applied is something Pinscher should
do automatically.

`pinscher lint-boot` would have caught this in three seconds. The bug ate four days.

## License

MIT