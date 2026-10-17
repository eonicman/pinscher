#!/bin/bash
# diagnose-missing-driver.sh — a device isn't working; find out why and what
# to do about it, walking the safety ladder from safe to risky.
#
# Usage: ./diagnose-missing-driver.sh <device>
#   <device> can be a PCI address, vendor:device ID, driver name, or a
#   free-text description ("FirePro", "amdgpu", "02:00.0", "1002:6799").
set -euo pipefail

DEVICE="${1:?usage: $0 <device>}"

echo "== Diagnosis =="
pinscher diagnose "$DEVICE"

echo
echo "== Known quirks for this device =="
pinscher quirks "$DEVICE" || echo "(none in the embedded quirk database)"

echo
echo "== Synthesize a fix (rung 1: safe quirk/param/udev generation only) =="
pinscher synthesize "$DEVICE" --rung 1

echo
echo "No luck? Escalate rung by rung:"
echo "  pinscher synthesize $DEVICE --rung 2   # userspace driver proposal (libusb/VFIO)"
echo "  pinscher synthesize $DEVICE --rung 3   # kernel source analysis"
echo "Rung 4 (novel kernel module synthesis) is sandbox-only — not run by this script."
