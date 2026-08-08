# Pinscher

Hardware orchestration layer — a persistent hardware intelligence layer that sits
between the kernel and OS-level AI agents. It discovers hardware, diagnoses driver
failures, and exposes a structured API for querying, configuring, and grouping
subsystems by intent.

**Status:** Phase 1–2 complete. See [CONCEPT.md](CONCEPT.md) for the full design
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
Probe all hardware and print a structured report.

```bash
# Human-readable
pinscher report

# JSON for scripting/AI consumption
pinscher report --json
```

### `pinscher lint-boot`
The killer feature — check that your initramfs contains all drivers needed for
boot-critical hardware. This is the check that would have caught the 4-day
initramfs bug that birthed Pinscher.

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
- **Phase 3** 🔜 — Driver synthesis / LLM integration for unknown hardware
- **Phase 4** 🔜 — Dynamic subsystem grouping (intent-based orchestration)

## Origin Story

Born from a real problem: CachyOS ISO built on a cloud VPS had `autodetect` in
mkinitcpio HOOKS, which stripped USB host controller drivers from the initramfs.
The Mac Pro 6,1 couldn't boot the live ISO — "Cannot open access to console, the
root account is locked." Every manual fix we applied is something Pinscher should
do automatically.

`pinscher lint-boot` would have caught this in three seconds. The bug ate four days.

## License

MIT