#!/system/bin/sh
# shellcheck shell=ash

MODDIR=${0%/*}
export HOME="$MODDIR"
export TMPDIR="$MODDIR/tmp"
export TZ="Asia/Shanghai"
export WB2A_SCHEDULER="0"

mkdir -p "$MODDIR/auths" "$MODDIR/data" "$MODDIR/logs" "$MODDIR/run" "$MODDIR/tmp"
chmod 0700 "$MODDIR/auths" "$MODDIR/data" "$MODDIR/logs" "$MODDIR/run" "$MODDIR/tmp"
chmod 0755 "$MODDIR/bin/wbapi" "$MODDIR/bin/wb2api-server" "$MODDIR/bin/wb2api-login"

"$MODDIR/bin/wbapi" start --quiet
