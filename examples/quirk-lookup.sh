#!/bin/bash
# quirk-lookup.sh — check the embedded quirk database for a device, and if
# nothing's there, generate a GitHub issue template to submit one.
#
# Usage: ./quirk-lookup.sh <device>
set -euo pipefail

DEVICE="${1:?usage: $0 <device>}"

echo "== Quirks for $DEVICE =="
if pinscher quirks "$DEVICE"; then
  exit 0
fi

echo
echo "Nothing in the embedded database yet. Generating a submission template:"
echo
pinscher quirk-submit "$DEVICE" --probe
echo
echo "Copy the above into a new issue at https://github.com/eonicman/pinscher/issues"
