#!/usr/bin/env bash
# Build a white-label storefront app for a specific merchant.
#
# Reads branding/<slug>/config.json + assets, stamps them into the Expo
# config, and produces a native binary via EAS for iOS + Android.
#
# Usage:
#   ./scripts/build-merchant.sh <merchant-slug> [profile]
#
#   merchant-slug   Folder under branding/ (e.g. "acme")
#   profile         EAS build profile (default: production)
#
# Examples:
#   ./scripts/build-merchant.sh acme
#   ./scripts/build-merchant.sh acme production-simulator
#
# Submitting:
#   MERCHANT_SLUG=acme eas submit --profile production --platform ios
#   MERCHANT_SLUG=acme eas submit --profile production --platform android
#
# To onboard a new merchant:
#   1. cp -r branding/example branding/<slug>
#   2. Edit branding/<slug>/config.json (name, bundle id, colors,
#      defaultStoreSlug, gipTenantId, easProjectId).
#   3. Drop merchant assets into branding/<slug>/assets/
#      (icon.png, adaptive-icon.png, splash.png).
#   4. Run this script.

set -euo pipefail

SLUG="${1:-}"
PROFILE="${2:-production}"

if [ -z "$SLUG" ]; then
  echo "usage: $0 <merchant-slug> [profile]" >&2
  exit 1
fi

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
CONFIG="$ROOT/branding/$SLUG/config.json"

if [ ! -f "$CONFIG" ]; then
  echo "error: $CONFIG does not exist." >&2
  echo "       cp -r $ROOT/branding/example $ROOT/branding/$SLUG and edit." >&2
  exit 1
fi

cd "$ROOT"
echo "→ Building storefront for merchant: $SLUG (profile: $PROFILE)"
MERCHANT_SLUG="$SLUG" eas build --profile "$PROFILE" --platform all
