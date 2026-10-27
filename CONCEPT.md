# Pinscher — Design Notes

> **Read this as a direction, not a feature list.** The [README](README.md)
> describes what exists and has been tested. This document describes where the
> project is trying to go, and much of it is not built. Sections are marked
> **Shipped**, **Partial**, or **Not built**.

## The idea

Pinscher is a Go binary that sits between the kernel and an OS-level AI agent.
It discovers hardware, explains why a driver isn't working, and exposes a
structured API an agent can use to query, configure, and eventually group
hardware subsystems by intent.

The aim is a persistent hardware layer rather than a boot-time lookup table.

## Origin

A custom CachyOS live ISO, built on a cloud VPS, had `autodetect` in the
mkinitcpio HOOKS. That stripped the USB host controller drivers from the
initramfs, so a Mac Pro 6,1 could not boot the ISO ("Cannot open access to
console, the root account is locked"). The problem was fully detectable before
the reboot: the hardware list and the initramfs contents were both known, and
nobody diffed them. `pinscher lint-boot` is that diff.

## Architecture

```
┌──────────────────────────────────────────┐
│  OS-level AI agent                       │
│  Expresses intent: "low-latency audio"   │
├──────────────────────────────────────────┤
│  Pinscher                                │
│  - probes hardware (PCI, USB, ACPI, ...) │
│  - diagnoses driver failures             │
│  - checks and sets module parameters     │
│  - exposes a JSON API (CLI + socket)     │
├──────────────────────────────────────────┤
│  Kernel + drivers + firmware             │
└──────────────────────────────────────────┘
```

## Phases

| Phase | Goal | Status |
|-------|------|--------|
| 1 | Probe and report hardware; `lint-boot`; local socket API | **Shipped** (`report`, `lint-boot`, `serve`) |
| 2 | Safe command interface: get/set module parameters, allow-list, audit log | **Shipped** (`get`, `set`, `apply`, `audit-log`) |
| 3 | Multi-bus probing, quirk database with citations | **Shipped** (PCI, USB, platform, ACPI, Bluetooth, WiFi; 22 quirks) |
| 4 | Driver synthesis: propose fixes up a safety ladder | **Partial** (proposals only, rungs 1–3; see below) |
| 5 | Watch mode and snapshot diff | **Shipped** (`watch`, `diff`) |
| 6 | Hardware profiles and recommendation | **Shipped** (`profile`) |
| 7 | Persistent daemon | **Shipped** (`serve`) |
| 8 | Dynamic subsystem grouping by intent | **Not built** |

### Driver synthesis and the safety ladder

The long-term idea is that when no driver or quirk exists for a device,
Pinscher could research it (find a similar driver in the kernel tree, reason
about what differs) and propose a fix. That is deliberately staged:

1. **Quirk, parameter or udev rule.** Safe. Proposals from the quirk database,
   optionally LLM-assisted via Ollama.
2. **Contained userspace driver.** Proposals only.
3. **Patched in-tree driver.** Risky. Searches a kernel source tree for similar
   drivers and proposes changes; nothing is patched, compiled or loaded.
4. **Novel kernel module.** Last resort. Not built.

An LLM-written kernel module is a way to turn a hallucination into a kernel
panic. The ladder exists so that each rung is proven, with boot-counting and
automatic fallback to a known-good boot entry, before the next is trusted.
That fallback mechanism is **not built**; `pinscher-boot-check.service`
currently only logs.

### Dynamic subsystem grouping (not built)

An agent says "I need a video encode pipeline", and Pinscher works out which
hardware is available (GPU encoder, CPU vector units, and so on), then
configures a coordinated group with shared scheduling priority. This is the
most speculative phase.

## Why look at the silicon

Hardware ships with a story: a model name and a feature list. Some of that is
silicon truth, some is product segmentation, and some is just driver defaults.
Reading what the device actually reports (IDs, capabilities, bound driver) is
more reliable than trusting its label. For example, a Mac Pro 6,1 documented
as having FirePro D300 GPUs reported D700-class Tahiti XT silicon in `lspci`,
which changes which quirk applies.

Areas where this kind of tool may add value:

- Known firmware bugs and ACPI workarounds, applied from a cited database.
- The same silicon sold under different SKUs with different features enabled.
- Parameters whose defaults are wrong for specific silicon.

## Possible long-term direction

If agents can express intent and Pinscher can reconfigure hardware safely, the
hardware layer could adapt to how a machine is used. That is a hypothesis, not
a result, and it depends on the safety work above.

Possible later targets are Linux-native VR/AR stacks (Monado-based), and,
much further out, locked-down headsets, where the barrier is the boot chain
rather than anything Pinscher does.

## Test hardware

- A 2013 Mac Pro 6,1 (Xeon E5-2697 v2, dual FirePro, Apple SMC, Fresco Logic
  FL1100 USB 3.0) on CachyOS, with a small spare partition to break freely.
- An Ubuntu 24.04 desktop (ASUS Maximus III GENE, i7-875K) for the
  Debian/Ubuntu code path. It has no VT-d, so it cannot test IOMMU or
  passthrough behaviour.
- A Dell OptiPlex 990 (i7-2600S, Q67 chipset, legacy BIOS, MBR disk, GRUB) on
  Ubuntu 26.04. Its IOMMU works (11 groups), so it can test passthrough of PCI
  devices other than a GPU; it has only the integrated Intel HD 2000 graphics.

## Name

A pinscher is a small, alert, purposeful dog. The name also nods to OpenClaw
and its family of small agent tools. It is Pinscher with an "s": the dog, not
the grip.

## Built by Bots

Pinscher is part of Built by Bots, a project where AI agents build real things
and keep receipts. The bug that started it cost about four days.
