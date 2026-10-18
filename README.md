# Pinscher

Hardware orchestration layer — a persistent hardware intelligence layer that sits
between the kernel and OS-level AI agents. It discovers hardware, diagnoses driver
failures, and exposes a structured API for querying, configuring, and grouping
subsystems by intent.

**Status:** v0.9.3, pre-1.0. Probing across six buses, `lint-boot`, diagnosis, a
22-entry quirk database, hardware profiles, watch and diff, a `serve` daemon
exposing the JSON API over a local Unix socket, and an opt-in driver-synthesis
proposer. See [CONCEPT.md](CONCEPT.md) for the longer-term design, which is
**ahead of** what is implemented.

**What has been tested on real hardware:** a 2013 Mac Pro 6,1 on CachyOS (Arch),
and an Ubuntu 24.04 desktop (ASUS Maximus III GENE, i7-875K). Anything else is
unverified. Bug reports from other hardware are the most useful thing you can
send; see [CONTRIBUTING.md](CONTRIBUTING.md).

Built by AI agents as part of the [Built by Bots](https://github.com/eonicman) project.

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

# Safe to post publicly: hostname replaced, MAC addresses keep only the vendor
# prefix, serial numbers redacted
pinscher report --json --anonymize
```

A plain report includes your hostname and, on machines with WiFi or Bluetooth,
full MAC addresses. Use `--anonymize` before attaching one to a public issue.
`quirk-submit` always anonymizes its output. `diagnose` output includes kernel
log excerpts that `--anonymize` does not cover, so read it before posting.

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
(citation URL), severity, and verified status.

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

### `pinscher synthesize <device>`
Driver synthesis engine — proposes driver solutions using a safety ladder.
Rung 1 (safe): quirk/param/udev rules. Rung 2 (contained): userspace driver.
Rung 3 (risky): patched in-tree driver. Rung 4 (last resort): novel module.
Optionally queries Ollama for LLM-assisted synthesis.

```bash
# Rung 1 (default, safe)
pinscher synthesize 1002:6799

# Rung 2 (userspace driver proposals)
pinscher synthesize 1002:6799 --rung 2

# Skip LLM, database only
pinscher synthesize 1002:6799 --model none

# Use specific Ollama model
pinscher synthesize 1002:6799 --model qwen3:8b
```

### `pinscher watch`
Monitor hardware changes in real-time. Uses udevadm monitor for live events,
falling back to polling if udev is unavailable.

```bash
# Start watching (Ctrl+C to stop)
pinscher watch

# With initial snapshot
pinscher watch --snapshot

# JSON output for scripting
pinscher watch --json

# Stop after 30 seconds
pinscher watch --duration 30s
```

### `pinscher diff <file1> [file2]`
Compare two probe snapshots. If only one file is given, compare against the
current system. Shows added, removed, and changed devices.

```bash
# Create a snapshot
pinscher report --json > before.json

# Make hardware changes, then compare
pinscher diff before.json

# Compare two saved snapshots
pinscher diff old-server.json new-server.json

# JSON output
pinscher diff before.json --json
```

### `pinscher apply <profile>`
Apply multiple settings from a profile file (key=value format, comments allowed).

```bash
# Mac Pro 6,1 profile example
cat > /etc/pinscher/macpro61.profile <<EOF
# Mac Pro 6,1 hardware profile
amdgpu.si_support=1
amdgpu.dc=1
EOF

# Preview
pinscher apply /etc/pinscher/macpro61.profile --dry-run

# Apply
sudo pinscher apply /etc/pinscher/macpro61.profile
```

### `pinscher audit-log`
Print the audit log of all config operations (set, apply) performed this session.

### `pinscher serve`
Run Pinscher as a persistent daemon — the hardware intelligence layer from
CONCEPT.md. Exposes the same structured data as the CLI over a local Unix
socket using newline-delimited JSON, so agents and scripts can query live
hardware state without paying a cold process start per query.

Read-only methods: `ping`, `version`, `report`, `lint-boot`, `diagnose`,
`quirks`, `profile`, `profiles`, `recommend`, `list-params`, `get`,
`audit-log`. Mutating method: `set` — only available when started with
`--allow-write`, and the socket is always created with 0600 permissions
(owner only).

```bash
# Read-only daemon on the default socket
pinscher serve

# Custom socket, quiet
pinscher serve --socket=/tmp/pinscher.sock -q

# Enable the mutating "set" method (socket is 0600 — owner only)
sudo pinscher serve --allow-write

# One-shot query from the shell
echo '{"id":1,"method":"report"}' | socat - UNIX-CONNECT:/run/pinscher.sock
```

Wire protocol:

```
→ {"id":1,"method":"diagnose","params":{"device":"amdgpu"}}
← {"id":1,"ok":true,"result":{"status":"ok","checks":null,...}}
← {"id":2,"ok":false,"error":"unknown method: frobnicate (supported: ping, version, ...)"}
```

Python client (10 lines, no dependencies):

```python
import socket, json

def pinscher(method, **params):
    s = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
    s.connect("/run/pinscher.sock")
    f = s.makefile("r")
    s.sendall((json.dumps({"id": 1, "method": method, "params": params}) + "\n").encode())
    resp = json.loads(f.readline())
    s.close()
    if not resp["ok"]:
        raise RuntimeError(resp["error"])
    return resp["result"]

report = pinscher("report")
quirks = pinscher("quirks", device="1002:6799")
```

Behavior notes:

- Responses are byte-identical to the equivalent `--json` CLI output.
- Numeric params keep their exact source text (`120000` never becomes `1.2e+05`).
- Empty lists may serialize as `null` — treat `null` as `[]` (same as CLI JSON).
- A stale socket file from an unclean shutdown is reclaimed automatically;
  starting a second live instance on the same socket is refused.

### `pinscher version`
Print the version. Also available as `pinscher --version` or `pinscher -V`.

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
fixes, each with provenance (citation URL) and verified status. **22 entries across
9 categories** covering Mac Pro, ThinkPad, Framework, Dell, NVIDIA, AMD, Raspberry Pi,
and more.

### Key Quirks by Severity

| Severity | Module | Quirk | Description |
|----------|--------|-------|-------------|
| **Fatal** | amdgpu | tahiti-xt-si-support | Tahiti XT needs si_support=1 |
| **Fatal** | nvme | samsung-970evo-plus-firmware | Samsung 970 EVO+ firmware bug causes I/O errors |
| **Warning** | amdgpu | tahiti-xt-dc | Tahiti XT needs dc=1 for display |
| **Warning** | thunderbolt | thinkpad-t480s-thunderbolt-power | T480s/T490 Thunderbolt firmware update needed |
| **Warning** | thunderbolt | dell-xps-thunderbolt | Dell XPS 13 Thunderbolt firmware update |
| **Warning** | iwlwifi | intel-ax210-power-save | Intel AX200/210 WiFi power save causes disconnects |
| **Warning** | r8169 | realtek-rtl8125-aspm | RTL8125 2.5GbE ASPM causes link drops |
| **Warning** | nvme | nvme-apst-disable | NVMe APST crashes on some SSDs |
| **Info** | snd_hda_intel | Various audio codec fixups | Framework, Realtek, Intel HDA |
| **Info** | xhci_hcd | FL1100/ASM1142 USB issues | USB controller quirks |
| **Info** | applesmc, apple-gmux | Mac Pro 6,1 | Apple hardware quirks |
| **Info** | power | framework-13-s3-sleep | Framework needs S3 sleep |
| **Info** | power | rpi4-usb-c-power | RPi4 USB-C charger compatibility |

Full list: `pinscher quirks` or `pinscher quirks --json`

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
- **Phase 4** ✅ — Driver synthesis engine (safety ladder, LLM integration via Ollama)
- **Phase 5** ✅ — Watch mode + Diff command (real-time hardware monitoring, snapshot comparison)
- **Phase 6** ✅ — Profile system (v0.7.0): built-in presets, hardware-matched recommendation, quirk enrichment
- **Phase 7** ✅ — Serve daemon (v0.9.0): persistent Unix-socket JSON API — the hardware intelligence layer
- **Phase 8** 🔜 — Dynamic subsystem grouping (intent-based orchestration)

## Origin Story

Born from a real problem: a custom CachyOS live ISO, built on a cloud VPS, had
`autodetect` in the mkinitcpio HOOKS, which stripped USB host controller
drivers from the initramfs. The Mac Pro 6,1 couldn't boot the live ISO —
"Cannot open access to console, the root account is locked." Every manual fix
we applied is something Pinscher should do automatically.

`pinscher lint-boot` is meant to catch exactly this before the reboot. The bug
cost about four days.

## Contributing and contact

See [CONTRIBUTING.md](CONTRIBUTING.md). Issues and PRs are welcome. For
anything that doesn't fit an issue, write to ghost@eonic.cloud.

## License

MIT

## Changelog

See [CHANGELOG.md](CHANGELOG.md) for version history.