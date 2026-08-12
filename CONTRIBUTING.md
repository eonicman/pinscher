# Contributing to Pinscher

Pinscher is a [Built by Bots](https://github.com/eonicman) project — built by AI agents, for AI agents (and the humans who love them).

## How to Contribute

### Quirk Database

The most valuable contribution is **hardware quirks**. If you've ever had to add a kernel parameter, create a modprobe config, or search for hours why your hardware doesn't work on Linux — that's a quirk.

Each quirk needs:

1. **ID**: kebab-case, descriptive (e.g., `amdgpu-tahiti-xt-si-support`)
2. **Device IDs**: PCI vendor:device IDs (find with `lspci -nn` or `pinscher report --json`)
3. **Device Names**: Human-readable names
4. **Category**: gpu, usb, storage, audio, network, apple, power, input, thunderbolt
5. **Module**: Kernel module affected (e.g., `amdgpu`, `xhci_hcd`)
6. **Params**: Module parameters to set (e.g., `{"si_support": "1"}`)
7. **Description**: What the quirk is
8. **Reason**: Why the parameter is needed (the war story)
9. **Citation**: URL or reference to the source (bug report, wiki, LKML thread)
10. **Severity**: fatal (won't boot/work without it), warning (degraded without it), info (good to know)
11. **Verified**: `true` only if you've tested it on real hardware

Add your quirk to `internal/quirk/quirk.go` in the `Database.Quirks` slice, then run:

```bash
go test ./internal/quirk/
go build ./...
./pinscher quirks
```

### Code Contributions

1. Fork the repo
2. Create a feature branch (`git checkout -b feat/my-feature`)
3. Write code + tests
4. Ensure `go build ./...` and `go test ./...` pass
5. Commit with conventional commit messages (`feat:`, `fix:`, `docs:`)
6. Open a PR

### Reporting Hardware Issues

Run `pinscher report --json > my-system.json` and `pinscher diagnose <device> --json > diagnosis.json` and include both in your issue.

## Development Setup

```bash
git clone https://github.com/eonicman/pinscher.git
cd pinscher
go build -o pinscher ./cmd/pinscher/
go test ./...
```

Requires Go 1.21+. No external dependencies for core functionality (Ollama is optional for `synthesize`).

## Architecture

```
cmd/pinscher/          CLI entry point
internal/
  config/              Module parameter get/set/apply with safety checks
  diagnose/            Deep device diagnosis (driver status, dmesg, quirks)
  lintboot/            Initramfs coverage check (the killer feature)
  probe/               Hardware probing (PCI, USB, platform, ACPI, BT, WiFi)
  quirk/               Embedded quirk database
  synthesize/          Driver synthesis engine (safety ladder + LLM)
  watch/               Real-time hardware change monitoring (udev + polling)
```

## License

MIT