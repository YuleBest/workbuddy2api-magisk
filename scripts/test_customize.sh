#!/bin/sh
set -eu

ROOT="$(cd -- "$(dirname "$0")/.." && pwd)"
BASE="$(mktemp -d)"
trap 'rm -rf "$BASE"' EXIT HUP INT TERM

ui_print() {
    printf '%s\n' "$*"
}
abort() {
    printf 'abort: %s\n' "$*" >&2
    exit 1
}
set_perm() {
    printf '%s\n' "$1" >>"$PERM_LOG"
}

run_installer() {
    modpath="$1"
    old_moddir="$2"
    PERM_LOG="$3"
    test_script="$BASE/customize-$4.sh"
    sed "s#/data/adb/modules/wb2api#$old_moddir#g" \
        "$ROOT/module/customize.sh" >"$test_script"
    (
        KSU=true
        ARCH=arm64
        API=23
        MODPATH="$modpath"
        export KSU ARCH API MODPATH PERM_LOG
        # shellcheck disable=SC1090
        . "$test_script"
    )
}

prepare_module() {
    modpath="$1"
    mkdir -p "$modpath/bin"
    cp -a "$ROOT/module/." "$modpath/"
    cp -a "$ROOT/build/stage/bin/." "$modpath/bin/"
}

fresh="$BASE/fresh"
fresh_old="$BASE/fresh-old"
fresh_perms="$BASE/fresh-perms.log"
mkdir -p "$fresh_old"
prepare_module "$fresh"
run_installer "$fresh" "$fresh_old" "$fresh_perms" fresh >/dev/null

test -f "$fresh/config.json"
test -f "$fresh/keys.json"
test ! -f "$fresh/config.android.json"
if grep -q '__API_KEY__' "$fresh/config.json"; then
    printf 'fresh config still contains the API key placeholder\n' >&2
    exit 1
fi
grep -q "^$fresh/config.json$" "$fresh_perms"
grep -q "^$fresh/keys.json$" "$fresh_perms"
grep -q "^$fresh/bin/wbapi$" "$fresh_perms"

upgrade="$BASE/upgrade"
upgrade_old="$BASE/upgrade-old"
upgrade_perms="$BASE/upgrade-perms.log"
mkdir -p "$upgrade_old/auths" "$upgrade_old/data" "$upgrade_old/logs"
printf '{"listen":"127.0.0.1:7863","api_key":"old-key"}\n' >"$upgrade_old/config.json"
printf '{"keys":[{"key":"old-key"}]}\n' >"$upgrade_old/keys.json"
printf '{"account":"preserved"}\n' >"$upgrade_old/auths/workbuddy-test.json"
printf '{"state":"preserved"}\n' >"$upgrade_old/data/state.json"
printf 'preserved log\n' >"$upgrade_old/logs/gateway.log"
prepare_module "$upgrade"
run_installer "$upgrade" "$upgrade_old" "$upgrade_perms" upgrade >/dev/null

cmp "$upgrade_old/config.json" "$upgrade/config.json"
cmp "$upgrade_old/keys.json" "$upgrade/keys.json"
cmp "$upgrade_old/auths/workbuddy-test.json" "$upgrade/auths/workbuddy-test.json"
cmp "$upgrade_old/data/state.json" "$upgrade/data/state.json"
cmp "$upgrade_old/logs/gateway.log" "$upgrade/logs/gateway.log"
test ! -f "$upgrade/config.android.json"
grep -q "^$upgrade/config.json$" "$upgrade_perms"
grep -q "^$upgrade/keys.json$" "$upgrade_perms"

printf 'customize.sh tests passed: fresh + upgrade\n'
