#!/usr/bin/env bash
set -euo pipefail

VERSION="${VERSION:-0.1.0-rc.1}"
COMMIT="${COMMIT:-$(git rev-parse HEAD)}"
DATE="${DATE:-$(git show -s --format=%cI "$COMMIT")}"
TARGET_OS="${TARGET_OS:-$(go env GOOS)}"
TARGET_ARCH="${TARGET_ARCH:-$(go env GOARCH)}"
OUT_DIR="${OUT_DIR:-dist}"

case "$TARGET_OS" in
  linux|windows|darwin) ;;
  *)
    echo "unsupported TARGET_OS: $TARGET_OS" >&2
    exit 2
    ;;
esac

case "$TARGET_ARCH" in
  amd64|arm64) ;;
  *)
    echo "unsupported TARGET_ARCH: $TARGET_ARCH" >&2
    exit 2
    ;;
esac

mkdir -p "$OUT_DIR"
work_dir="$(mktemp -d)"
trap 'rm -rf "$work_dir"' EXIT

binary="ai"
if [[ "$TARGET_OS" == "windows" ]]; then
  binary="ai.exe"
fi

ldflags="-s -w"
ldflags+=" -X github.com/FernandoVillegas27/ai-engineering-os/core/buildinfo.Version=$VERSION"
ldflags+=" -X github.com/FernandoVillegas27/ai-engineering-os/core/buildinfo.Commit=$COMMIT"
ldflags+=" -X github.com/FernandoVillegas27/ai-engineering-os/core/buildinfo.Date=$DATE"

echo "Building AI Engineering OS $VERSION for $TARGET_OS/$TARGET_ARCH from $COMMIT"
CGO_ENABLED=0 GOOS="$TARGET_OS" GOARCH="$TARGET_ARCH"   go build -trimpath -buildvcs=false -ldflags "$ldflags"   -o "$work_dir/$binary" ./cmd/ai

package="ai-engineering-os_${VERSION}_${TARGET_OS}_${TARGET_ARCH}.tar.gz"
tar -C "$work_dir" -czf "$OUT_DIR/$package" "$binary"

if command -v sha256sum >/dev/null 2>&1; then
  (cd "$OUT_DIR" && sha256sum "$package" > "$package.sha256")
elif command -v shasum >/dev/null 2>&1; then
  (cd "$OUT_DIR" && shasum -a 256 "$package" > "$package.sha256")
else
  echo "no SHA-256 utility found" >&2
  exit 3
fi

echo "$OUT_DIR/$package"
cat "$OUT_DIR/$package.sha256"
