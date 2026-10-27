# Pinscher Changelog

## Unreleased

- **Tested on a third real machine:** a Dell OptiPlex 990 (i7-2600S, Q67
  chipset, legacy BIOS) on Ubuntu 26.04: 5 devices checked, 1 warning (`i915` is
  not in the initramfs, which is accurate: the kernel log shows the integrated GPU
  driver loading after the root filesystem is mounted). The anonymized report
  hid the hostname and MAC addresses as intended.
- Internal cleanup from `staticcheck`: removed two unused functions, compile the
  USB device-name pattern once instead of per device, dropped redundant
  `fmt.Sprintf` calls. No behaviour change.

## v0.9.4 — lint-boot actually checks PCI devices now (important fix)

**If you ran v0.9.3 or earlier, `lint-boot` was not checking your PCI hardware.**
On real systems it skipped every storage, USB, GPU and network device and could
still print "All clear". It was found on 2026-10-19 by running it as a normal
user on two real machines and reading the numbers ("1 boot-critical device
checked" on a machine with SATA, USB, graphics and Ethernet). Nothing in the
test suite covered it. Everything below was found that way and now has
regression tests.

- **PCI class codes kept sysfs's `0x` prefix** (`0x010601`), but `lint-boot`'s
  table of boot-critical classes has no prefix, so no PCI device ever matched.
  An early commit ("fix vendor/device ID normalization") dropped the
  normalization for the class field only. Fixed in one place
  (`probe.NormalizePCIClass`). Side effects fixed too: class names in the
  report (`Storage Controller`, ...), `profile` class matching and
  `--assert class:...` all depend on the same field.
- **Built-in drivers were flagged as missing.** The built-in check was a
  substring match on the underscore name (`ehci_pci`) against a list that spells
  it with a dash (`ehci-pci.ko`), so a built-in driver produced a false FATAL
  ("keyboard may be dead at LUKS prompt") on a machine that boots fine. It now
  compares whole names and treats `-` and `_` as equal.
- **Alias matches that never bind were flagged.** `modprobe --resolve-alias`
  also lists sibling drivers (for example `amdgpu` for an old Radeon that uses
  `radeon`). If the driver bound at runtime is among the matches, only that one
  is checked.
- **Arch-family kernels with a package suffix were not found.**
  `/boot/initramfs-linux-cachyos.img` (and `linux-lts`, `linux-zen`, ...) gave
  "no initramfs found". Lookup now uses the kernel's `pkgbase`, then falls back
  to a single unambiguous `/boot/initramfs-*.img`.
- **On Arch/CachyOS no modules were ever listed.** `lint-boot` ran
  `lsinitcpio -a`, which prints a summary without file paths, so every module
  looked missing. It now runs plain `lsinitcpio`.
- **Fix suggestions matched the wrong distro.** Ubuntu/Debian images
  (`initrd.img-*`) now get `/etc/initramfs-tools/modules` +
  `update-initramfs -u`; Arch images keep the mkinitcpio text.
- **README exit-code note corrected.** `lint-boot` exits 1 for any issue found
  in human mode (not only FATAL), and `--json` mode does not set the exit code
  (known gap, unchanged; use `--assert` to gate).

Re-checked on real hardware after the fixes: Ubuntu 24.04 (6 devices checked, 1
true warning: `radeon` is not in the initramfs) and CachyOS on a Mac Pro 6,1 (11
devices checked, 2 true warnings: `tg3`, `wl`). A deliberately stripped test
initramfs produces the expected FATAL storage and USB findings.

### Also in this release (earlier unreleased work)


- **New: `pinscher report --anonymize`.** A plain report contains the hostname
  and, on machines with WiFi or Bluetooth, full MAC addresses, and
  CONTRIBUTING asked people to paste reports into public issues. `--anonymize`
  replaces the hostname, keeps only the vendor prefix of each MAC address
  (`4c:74:bf:xx:xx:xx`), and redacts any capability whose key mentions
  "serial". Hardware IDs, drivers and descriptions are untouched. Opt-in for
  `report`, so saved snapshots and `diff` behave as before.
- **Changed: `quirk-submit` output is always anonymized.** Its issue template
  previously always printed the hostname. It is meant to be posted publicly.
- **Not covered:** `diagnose` output includes kernel log excerpts that are not
  anonymized, and the `serve` daemon's `report` method is unchanged. Both are
  noted in the docs.
- **Fix: `internal/serve` test teardown error.** `Server.Close()` returned
  `use of closed network connection` when `Serve` had already closed the
  listener on context cancellation. `Close()` now treats an already-closed
  listener as success. Reproduced on every run under
  `go test -race -count=100 ./internal/serve` before the fix; clean after.
  This was test teardown, not a daemon bug.

## v0.9.3 — fixes from the first real Debian/Ubuntu run

First live run of the Debian path (Ubuntu 24.04, an ASUS Maximus III GENE /
i7-875K desktop, 2026-10-18). Everything below was invisible on Arch and on
the dev VM.

- **`lint-boot` auto-detect now finds Debian/Ubuntu initramfs**
  (`/boot/initrd.img-<ver>`, `/boot/initrd.img`). Previously only Arch paths
  were checked, so every Debian-family host got "no initramfs found".
- **`lint-boot --assert x=y` no longer treats `x=y` as the initramfs path.**
  (Bug in v0.9.2's own argument loop.)
- **Device lookup accepts short PCI addresses** (`01:00.0`, as `lspci` prints)
  as well as full sysfs form (`0000:01:00.0`).
- Test fix: assert test picks a PCI device (an ACPI device broke it).
- Known at the time: `internal/serve` tests reported a `Close: use of closed
  network connection` cleanup error under `-race` (test teardown, not the
  daemon). Fixed afterwards; see Unreleased.

## v0.9.2 — `lint-boot --assert`

### New: `pinscher lint-boot --assert field:device=value`

Assert that a device's field (`driver`, `bus`, `vendor`, `device_id`, `class`,
`description`) equals a given value — e.g. confirm a GPU is bound to
`vfio-pci` for passthrough before trusting a boot. Repeatable; all assertions
run against one shared probe of the current system. A malformed assertion
string is a hard error (it's a typo); a device that can't be found is
reported as a failed assertion with `Error` set, so one bad query doesn't
hide every other assertion's result in the same run.

Device identifiers use the same resolution as `diagnose`/`quirk-submit` (PCI
path, vendor:device ID, driver name, or partial description match) — now a
single shared implementation (`probe.FindDevice`) instead of three slightly-
drifted copies.

**Exit code:** `--assert` failures always exit 1, in both `--json` and
human output. This is new, standalone behavior, not tied to the pre-existing
driver-missing checks — see the "known, not fixed here" note below.

```bash
pinscher lint-boot --assert "driver:0000:02:00.0=vfio-pci" --assert "driver:0000:06:00.0=amdgpu"
```

### Known gap, deliberately NOT fixed here

`lint-boot`'s pre-existing driver-missing checks only exit 1 in human-
readable mode — `--json` mode has never exited nonzero on failure, only the
`--assert` path above does. Looks like the same bug class as `diff`'s
exit-0-always gap (v0.8.1), but `lint-boot` has a real shipped consumer,
`dist/pinscher-boot-check.service` (a 3-step oneshot: `lint-boot --json` /
`report --json` / `profile recommend --json`), which currently relies on
none of those three steps failing the unit. Changing that silently could
stop the later two steps from running on any boot with so much as an
info-level finding. Needs a deliberate decision, not a quiet fix.

*(Also found while investigating this: the "boot-counter, auto-revert on
next boot" mechanism referenced in some design docs doesn't exist anywhere
in the real code or this systemd unit — `pinscher-boot-check.service` is
just a logger today, no gating/revert logic. Worth not repeating that
assumption elsewhere.)*

*Verified via unit tests (`internal/lintboot/assert_test.go`,
`cmd/pinscher/flags_test.go`). The CLI path itself couldn't be exercised
end-to-end on the dev host, which has no initramfs to lint against.*

---

## v0.9.1 — Serve Daemon + diff can fail

Merged divergent lines: the v0.9.0 serve daemon + the v0.8.1
`diff --fail-on-change` — both shipped separately from forks of v0.8.0;
combined here without losing either narrative.

## v0.8.1 — `diff` can actually fail

### New: `pinscher diff --fail-on-change`

`diff` previously always exited 0, even when it found real differences —
discovered while writing the fleet/enterprise docs (a passthrough-regression
check assumed `diff` could gate a reboot via `&&`/`||`; it couldn't, no
matter what flags existed). `--fail-on-change` exits 1 if any differences are
found; default behavior (always exit 0) is unchanged so existing scripts
aren't surprised by this release.

```bash
pinscher diff before.json after.json --fail-on-change || echo "something changed"
```

*Verified live on 2026-10-17: forced a driver-field diff between two real
snapshots and confirmed exit 0→1 only with the flag present, and that the
default path still exits 0. Whether this should become the default instead of
opt-in is an open question.*

---

## v0.8.0 — Quirk Submission, Boot Service Profile Recommendations

### New: `pinscher quirk-submit` command

Generates a ready-to-file GitHub issue template for a new quirk — device info,
existing quirks for context, and a proposed quirk entry.

- `--probe` for full hardware data in the template
- `--json` for programmatic use

### Boot service now runs profile recommendations

`pinscher-boot-check.service` runs `profile recommend` after `lint-boot` and
`report`, so a profile suggestion shows up automatically on boot alongside
the existing checks.

### Other
- Full usage docs for `watch`, `diff`, `profile`, and `quirk-submit`
- Examples section updated with all current command variants

---

## v0.7.0 — Hardware Profiles

### New: `pinscher profile` command

`list` / `show` / `apply` / `recommend` hardware settings profiles.

- 6 built-in profiles: gaming, low-power, audio-production, vr, server, macpro61
- Auto-recommendation based on detected hardware (driver, vendor/device ID, class, bus)
- Quirk-database enrichment via `--enrich`, merging quirk entries into profile settings
- Conflict detection (e.g. gaming conflicts with low-power)
- Blocked params are skipped with an explanation rather than silently applied
- Custom JSON profile files, plus legacy key=value files for backward compat
- `--dry-run` support, JSON output for all subcommands

---

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