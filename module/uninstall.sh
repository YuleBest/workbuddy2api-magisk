#!/system/bin/sh
# shellcheck shell=ash

MODDIR=${0%/*}
if [ -x "$MODDIR/bin/wbapi" ]; then
    "$MODDIR/bin/wbapi" stop --quiet >/dev/null 2>&1 || true
fi
