#!/bin/bash
# profile-recommend.sh — get a settings profile recommended for the current
# hardware, review it, then apply for real once it looks right.
#
# Built-in profiles: gaming, low-power, audio-production, vr, server, macpro61
set -euo pipefail

echo "== Recommended profile for this hardware =="
pinscher profile recommend

echo
echo "Review a specific profile before applying:"
echo "  pinscher profile <name>                    # show what it would change"
echo "  pinscher profile <name> --enrich            # merge in hardware-specific quirks"
echo "  pinscher profile <name> --apply --dry-run   # preview the actual apply"
echo "  pinscher profile <name> --apply             # apply for real"
