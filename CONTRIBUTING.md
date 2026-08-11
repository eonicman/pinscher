# Contributing to Pinscher

Thanks for your interest! Pinscher is a [Built by Bots](https://github.com/eonicman) project.

## Development

```bash
# Build
make build

# Run tests
make test

# Vet
make lint

# Quick local check (build + report + lint-boot)
make check

# Install to /usr/local/bin
sudo make install
```

## Adding Quirks

The quirk database is in `internal/quirk/quirk.go`. Each entry needs:

1. **ID**: kebab-case, descriptive (e.g. `amdgpu-tahiti-xt-si-support`)
2. **DeviceIDs**: PCI vendor:device IDs or platform device names
3. **Severity**: `fatal`, `warning`, or `info`
4. **Module**: kernel module name
5. **Params**: module parameters to set (can be empty for info-only quirks)
6. **Citation**: URL or reference to the source that justifies this quirk
7. **Verified**: set to `true` only if tested on real hardware

Every quirk must have a citation. No folklore — if you can't cite it, it's not a quirk, it's a guess.

## Adding Bus Probes

Each bus gets its own `probeXxx()` function in `internal/probe/probe.go`:

1. Walk the sysfs directory
2. Build a `Device` struct for each entry
3. Read attributes from sysfs files
4. Resolve parent bus devices for cross-references
5. Use `readSysFile()` for all sysfs reads (handles missing files gracefully)
6. Add tests in `probe_newbus_test.go`

## Reporting Issues

Please include:
- `pinscher report --json` output
- `pinscher lint-boot` output
- Kernel version (`uname -r`)
- Distribution

## License

By contributing, you agree that your contributions will be licensed under the MIT License.