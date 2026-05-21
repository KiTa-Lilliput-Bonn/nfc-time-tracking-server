#!/usr/bin/env bash
# Lädt das offizielle restic windows_amd64-ZIP plus LICENSE für den WiX-MSI-Build.
# Version: installer/windows/redist/restic-version.txt (eine Zeile, Semver ohne v).
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
REDIST="${REPO_ROOT}/installer/windows/redist"

VERSION="$(tr -d ' \t\r\n' <"${REDIST}/restic-version.txt")"
if [[ ! "${VERSION}" =~ ^[0-9]+\.[0-9]+\.[0-9]+ ]]; then
  echo "fetch-restic-redist: ungültige Version in restic-version.txt: '${VERSION}'" >&2
  exit 1
fi

TMP="$(mktemp -d)"
trap 'rm -rf "${TMP}"' EXIT

BASE="https://github.com/restic/restic/releases/download/v${VERSION}"
ZIP_NAME="restic_${VERSION}_windows_amd64.zip"

curl -fsSL "${BASE}/SHA256SUMS" -o "${TMP}/SHA256SUMS"
curl -fsSL "${BASE}/${ZIP_NAME}" -o "${TMP}/${ZIP_NAME}"

EXPECTED="$(grep -F " ${ZIP_NAME}" "${TMP}/SHA256SUMS" | awk '{print $1}' | head -1)"
if [[ ! "${EXPECTED}" =~ ^[a-fA-F0-9]{64}$ ]]; then
  echo "fetch-restic-redist: keine gültige SHA256-Zeile für ${ZIP_NAME} in SHA256SUMS" >&2
  exit 1
fi

sha256_hex() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  else
    shasum -a 256 "$1" | awk '{print $1}'
  fi
}

ACTUAL="$(sha256_hex "${TMP}/${ZIP_NAME}")"
if [[ "${ACTUAL}" != "${EXPECTED}" ]]; then
  echo "fetch-restic-redist: SHA256 mismatch für ${ZIP_NAME}" >&2
  echo "  erwartet: ${EXPECTED}" >&2
  echo "  ist:      ${ACTUAL}" >&2
  exit 1
fi

mkdir -p "${REDIST}"
unzip -q -o "${TMP}/${ZIP_NAME}" -d "${TMP}"
mv -f "${TMP}/restic_${VERSION}_windows_amd64.exe" "${REDIST}/restic.exe"

curl -fsSL "https://raw.githubusercontent.com/restic/restic/v${VERSION}/LICENSE" \
  -o "${REDIST}/restic-LICENSE"

echo "fetch-restic-redist: OK restic v${VERSION} → ${REDIST}/restic.exe + restic-LICENSE"
