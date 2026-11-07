#!/usr/bin/env bash
set -euo pipefail

VERSION="${VERSION:-0.1.0-rc.1}"
OUT_DIR="${OUT_DIR:-dist}"
GRADLE_BIN="${GRADLE_BIN:-gradle}"

case "$VERSION" in
  v*) VERSION="${VERSION#v}" ;;
esac

mkdir -p "$OUT_DIR"
OUT_DIR_ABS="$(cd "$OUT_DIR" && pwd)"

"$GRADLE_BIN" -p clients/intellij   -PreleaseVersion="$VERSION"   test buildPlugin verifyPluginProjectConfiguration   --no-daemon

mapfile -t distributions < <(find clients/intellij/build/distributions -maxdepth 1 -type f -name '*.zip' -print | sort)
if [[ "${#distributions[@]}" -ne 1 ]]; then
  echo "expected exactly one IntelliJ plugin distribution, found ${#distributions[@]}" >&2
  printf '%s\n' "${distributions[@]:-}" >&2
  exit 4
fi

package="ai-engineering-os_${VERSION}_intellij.zip"
cp "${distributions[0]}" "$OUT_DIR_ABS/$package"

if command -v sha256sum >/dev/null 2>&1; then
  (cd "$OUT_DIR_ABS" && sha256sum "$package" > "$package.sha256")
elif command -v shasum >/dev/null 2>&1; then
  (cd "$OUT_DIR_ABS" && shasum -a 256 "$package" > "$package.sha256")
else
  echo "no SHA-256 utility found" >&2
  exit 3
fi

echo "$OUT_DIR_ABS/$package"
cat "$OUT_DIR_ABS/$package.sha256"
