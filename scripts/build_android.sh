#!/bin/sh
set -eu

ROOT="$(cd -- "$(dirname "$0")/.." && pwd)"
NDK="${ANDROID_NDK_HOME:-/home/yule/opt/android-ndk-r27c}"
TOOLCHAIN="$NDK/toolchains/llvm/prebuilt/linux-x86_64/bin"
API=23
CC="$TOOLCHAIN/aarch64-linux-android${API}-clang"
STRIP="$TOOLCHAIN/llvm-strip"
VERSION="$(sed -n 's/^version=//p' "$ROOT/module/module.prop" | head -n 1)"
STAGE="$ROOT/build/stage"
ZIP="$ROOT/build/wb2api-${VERSION}-arm64-v8a.zip"

[ -x "$CC" ] || {
    printf 'Android NDK clang not found: %s\n' "$CC" >&2
    exit 1
}
[ -x "$STRIP" ] || {
    printf 'Android NDK llvm-strip not found: %s\n' "$STRIP" >&2
    exit 1
}

rm -rf "$STAGE"
mkdir -p "$STAGE/bin" "$ROOT/build"
cp -a "$ROOT/module/." "$STAGE/"

export GOOS=android
export GOARCH=arm64
export CGO_ENABLED=1
export CC

build() {
    output="$1"
    package="$2"
    (
        cd "$ROOT"
        go build -trimpath -buildmode=pie -tags netcgo \
            -ldflags="-s -w" -o "$output" "$package"
    )
    "$STRIP" --strip-unneeded "$output"
}

build "$STAGE/bin/wb2api-server" ./cmd/server
build "$STAGE/bin/wbapi" ./cmd/wbapi
build "$STAGE/bin/wb2api-login" ./cmd/login

chmod 0755 "$STAGE"/bin/*
chmod 0755 "$STAGE"/customize.sh "$STAGE"/service.sh "$STAGE"/action.sh "$STAGE"/uninstall.sh

rm -f "$ZIP" "$ZIP.sha256"
(
    cd "$STAGE"
    zip -9 -r "$ZIP" .
)
(
    cd "$ROOT/build"
    sha256sum "$(basename "$ZIP")" >"$(basename "$ZIP").sha256"
)

printf 'Built: %s\n' "$ZIP"
printf 'SHA-256: %s\n' "$ZIP.sha256"
