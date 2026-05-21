#!/usr/bin/env bash
# Patches WiX Package Version (four-part) and MSI OutputName from a semver tag or plain semver.
# Usage: patch-wix-version.sh v1.2.3   OR   patch-wix-version.sh 1.2.3
# Rules: v1 -> 1.0.0.0, v1.2 -> 1.2.0.0, v1.2.3 -> 1.2.3.0. Pre-releases (e.g. 1.0.0-beta) are rejected.
set -euo pipefail

ver_input="${1:?usage: patch-wix-version.sh <vX.Y.Z|X.Y.Z>}"
ver="${ver_input#v}"
if [[ ! "$ver" =~ ^[0-9]+(\.[0-9]+){0,2}$ ]]; then
  echo "patch-wix-version: invalid version '$ver_input' (expected vX.Y.Z or X.Y.Z with numeric segments only)" >&2
  exit 1
fi

IFS='.' read -r p1 p2 p3 <<<"$ver"
major="${p1:-0}"
minor="${p2:-0}"
patch="${p3:-0}"
wix_four="${major}.${minor}.${patch}.0"
output_suffix="${major}.${minor}.${patch}"

repo_root="${GITHUB_WORKSPACE:-}"
if [[ -z "$repo_root" ]]; then
  repo_root="$(cd "$(dirname "$0")/../.." && pwd)"
fi

package_wxs="${repo_root}/installer/windows/wix/Package.wxs"
wixproj="${repo_root}/installer/windows/wix/NfcTimeTracking.wixproj"

if [[ ! -f "$package_wxs" ]] || [[ ! -f "$wixproj" ]]; then
  echo "patch-wix-version: missing $package_wxs or $wixproj" >&2
  exit 1
fi

# Do not match InstallerVersion="…" (substring "Version=" inside the attribute name).
perl -i -pe 's/(?<!Installer)Version="[^"]+"/Version="'"${wix_four}"'"/' "$package_wxs"
perl -i -pe 's#<OutputName>NFC-Time-Tracking-[^<]+</OutputName>#<OutputName>NFC-Time-Tracking-'"${output_suffix}"'</OutputName>#' "$wixproj"

echo "patch-wix-version: Package Version=${wix_four}, OutputName suffix=${output_suffix}"
