#!/bin/bash
# lint-boot-before-reboot.sh — the check that would have saved 4 days.
#
# Run this before every kernel update / initramfs regen / hardware change.
# It catches the class of bug Pinscher was built to prevent: a boot-critical
# driver silently missing from the initramfs, discovered only after reboot
# when you can't get a keyboard or storage controller to respond.
set -euo pipefail

echo "== Full hardware report =="
pinscher report

echo
echo "== Boot-coverage lint =="
if pinscher lint-boot; then
  echo "OK — every boot-critical device resolves to a module present in the initramfs."
else
  echo "FATAL — a boot-critical driver is missing. Fix this before you reboot." >&2
  exit 1
fi
