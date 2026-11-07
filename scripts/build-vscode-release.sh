#!/usr/bin/env bash
set -euo pipefail

VERSION="${VERSION:-0.1.0-rc.1}"
OUT_DIR="${OUT_DIR:-dist}"
VSCE_VERSION="${VSCE_VERSION:-4.0.0}"

case "$VERSION" in
  v*) VERSION="${VERSION#v}" ;;
esac

mkdir -p "$OUT_DIR"
OUT_DIR_ABS="$(cd "$OUT_DIR" && pwd)"
work_dir="$(mktemp -d)"
trap 'rm -rf "$work_dir"' EXIT

cp -R clients/vscode/. "$work_dir/"

node - "$work_dir/package.json" "$VERSION" <<'NODE'
const fs = require("node:fs");
const [path, version] = process.argv.slice(2);
const pkg = JSON.parse(fs.readFileSync(path, "utf8"));
pkg.version = version;
fs.writeFileSync(path, JSON.stringify(pkg, null, 2) + "\n");
NODE

package="ai-engineering-os_${VERSION}_vscode.vsix"

(
  cd "$work_dir"
  npx --yes "@vscode/vsce@${VSCE_VERSION}" package     --no-dependencies     --out "$OUT_DIR_ABS/$package"
)

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
