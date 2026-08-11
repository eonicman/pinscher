# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.4.0] - 2026-08-09

### Added
- Platform bus probe (`/sys/bus/platform/devices/`) — Apple SMC, RTC, sensors
- ACPI bus probe (`/sys/bus/acpi/devices/`) — power buttons, embedded controllers, thermal zones
- Filters 316 generic `device:*` ACPI containers (590 → 274 devices on Mac Pro 6,1)
- Bluetooth HCI probe (`/sys/class/bluetooth/`) — type, HCI/LMP version, features, manufacturer
- Resolves Bluetooth vendor/product from USB parent via `filepath.EvalSymlinks`
- WiFi PHY probe (`/sys/class/ieee80211/`) — MAC, HT/VHT, interface modes, cipher suites
- WiFi detailed capabilities via `iw phy# info` (bands, modes, frequencies)
- Resolves WiFi PCI parent for vendor/device IDs and description lookup
- `Device.Capabilities` map for bus-specific key-value pairs
- PCI/USB class name lookups (40+ PCI classes, 20+ USB classes)
- Platform/ACPI boot-critical checks in `lint-boot` (power button, SMC, embedded controller)
- `probe_newbus_test.go` — tests for all new bus probes

### Changed
- Use `filepath.EvalSymlinks()` for Bluetooth USB parent resolution (fixes string-based symlink resolution)
- WiFi MAC address preserved in `capabilities["mac_address"]` when replaced by PCI device ID
- ACPI filtering skips `device:*` entries — they're generic containers, not real hardware
- Platform/ACPI lint-boot uses modalias prefix matching (e.g. `acpi:PNP0C0C` → "input")

### Fixed
- Bluetooth vendor resolution with `EvalSymlinks` vs `filepath.Join` for sysfs symlinks
- WiFi PCI parent extraction handles multi-segment device link paths correctly

## [0.3.0] - 2026-08-08

### Added
- PCI ID database lookup (`probe/pciids.go`) — lazy-loads `/usr/share/hwdata/pci.ids`
- Human-readable device names for PCI devices (e.g. "Tahiti XT [Radeon HD 7970]" instead of "0x1002:0x6798")
- `pinscher diagnose` — deep diagnosis of a single device by PCI address, vendor:device ID, driver name, or description substring
- `pinscher quirks` — list known hardware quirks from embedded database
- `pinscher get <module> <param>` — read kernel module parameters
- `pinscher set <module> <param> <value>` — write module parameters with safety checks
- `pinscher list-params <module>` — list all parameters for a module
- `pinscher apply <profile>` — apply settings from a key=value profile file
- `pinscher audit-log` — view audit log of all config operations
- Quirk database with provenance (7 entries: amdgpu Tahiti XT, FL1100, Apple SMC, gmux, NVMe APST, snd-hda-intel)
- Tiered safety model: Safe / Warning / Blocked
- Dry-run mode for `set` and `apply`
- PKGBUILD for Arch Linux packaging
- systemd service for boot-time lint checks
- mkinitcpio hook for post-generation lint

### Changed
- Vendor/device ID normalization: stripped `0x` prefix for consistent matching
- Modalias multi-line bug fix: `resolveModalias` returns individual modules
- Dedup in `lint-boot`: same module from two GPUs no longer shows duplicate warnings
- Cleaned up stale root-level files from Phase 1

### Fixed
- Modalias resolution returns individual modules instead of concatenated multi-line output

## [0.2.0] - 2026-08-07

### Added
- Phase 2: diagnose, quirk database, set/get, audit log
- Quirk database with citations and severity levels
- Safety-tiered config writes (Safe, Warning, Blocked)
- Profile application with key=value format

## [0.1.0] - 2026-08-06

### Added
- Initial release: hardware probe + boot lint
- `pinscher report` — probe PCI and USB devices
- `pinscher lint-boot` — check initramfs coverage for boot-critical drivers
- JSON output mode for all commands
- Basic test coverage

[0.4.0]: https://github.com/eonicman/pinscher/releases/tag/v0.4.0
[0.3.0]: https://github.com/eonicman/pinscher/releases/tag/v0.3.0
[0.2.0]: https://github.com/eonicman/pinscher/releases/tag/v0.2.0
[0.1.0]: https://github.com/eonicman/pinscher/releases/tag/v0.1.0