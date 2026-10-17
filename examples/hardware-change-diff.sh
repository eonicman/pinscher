#!/bin/bash
# hardware-change-diff.sh — snapshot before a hardware/BIOS/kernel change,
# then diff against a snapshot taken after, to see exactly what moved.
#
# Usage:
#   ./hardware-change-diff.sh before      # run before the change
#   ./hardware-change-diff.sh after       # run after the change, prints the diff
set -euo pipefail

MODE="${1:?usage: $0 [before|after]}"
SNAPSHOT_DIR="${PINSCHER_SNAPSHOT_DIR:-/tmp/pinscher-snapshots}"
mkdir -p "$SNAPSHOT_DIR"

case "$MODE" in
  before)
    pinscher report --json > "$SNAPSHOT_DIR/before.json"
    echo "Snapshot saved to $SNAPSHOT_DIR/before.json — now make your hardware/BIOS/kernel change."
    ;;
  after)
    if [ ! -f "$SNAPSHOT_DIR/before.json" ]; then
      echo "No before-snapshot found — run '$0 before' first." >&2
      exit 1
    fi
    echo "== What changed =="
    pinscher diff "$SNAPSHOT_DIR/before.json"
    ;;
  *)
    echo "usage: $0 [before|after]" >&2
    exit 1
    ;;
esac
