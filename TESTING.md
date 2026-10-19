# Become a Pinscher tester in 5 minutes

Pinscher has been tested on **two machines**. We would like to make that twenty,
and the quickest way is for you to run it on yours and tell us what happened,
including when it works fine. You do not need to be a kernel developer.

## What you need
- A Linux machine (any distro; Arch-family and Debian/Ubuntu-family are the ones we have run on).
- Go 1.21 or newer, to build it (`go version`). There are no prebuilt binaries yet.

## Steps
```bash
git clone https://github.com/eonicman/pinscher.git
cd pinscher
go build -o pinscher ./cmd/pinscher/

./pinscher version
./pinscher lint-boot                         # compares your initramfs to your hardware
./pinscher report --json --anonymize > my-system.json
```
Then open a [Hardware test report](https://github.com/eonicman/pinscher/issues/new?template=hardware-report.yml)
and paste the output. If something looks wrong, say what you expected.

## What is safe to run
These **only read** from your system: `report`, `lint-boot`, `diagnose`, `quirks`,
`list-params`, `get`, `watch`, `diff`, `profile recommend`, `version`.

These **change** your system, so please do not run them while testing unless you
know what they do: `set`, `apply`, `profile apply`, and `serve --allow-write`.

`lint-boot` may need to read your initramfs under `/boot`; on most distros that
file is world-readable. If yours is not, `sudo ./pinscher lint-boot` is still read-only.

## Reading `lint-boot`
- **FATAL**: a storage or USB host driver your hardware needs is missing from the
  initramfs. This is the "won't boot / dead keyboard at the LUKS prompt" class.
- **WARNING**: a GPU or network driver is not in the initramfs. Often harmless
  (the driver loads later), but worth knowing.
- Built-in kernel drivers are counted as present.
- `lint-boot` does not check firmware blobs, kernel parameters or initramfs hooks.
  An "All clear" means "the drivers it knows to look for are present", not "your
  system will boot".

## Privacy
`report --anonymize` replaces your hostname, keeps only the vendor prefix of MAC
addresses and redacts serial numbers. `diagnose` output can contain kernel log
lines that are not anonymized, so skim it before posting.

## Known rough edges
- The human `report` marks many ACPI/platform entries as "NO DRIVER". That is
  normal for those entries and is noisy today.
- `lint-boot` exits 1 for any issue in human mode, and `--json` does not set the
  exit code.
- Fix suggestions assume mkinitcpio (Arch-family) or initramfs-tools
  (Debian/Ubuntu-family); dracut (Fedora/openSUSE) images cannot be listed yet.

Thank you. Reports are how this gets better.
