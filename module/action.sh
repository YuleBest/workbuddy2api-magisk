#!/system/bin/sh
# shellcheck shell=ash

MODDIR=${0%/*}
BIN="$MODDIR/bin/wbapi"
ADMIN_URL="http://127.0.0.1:7863/admin/"

"$BIN" start --quiet || exit 1
"$BIN" panel --show-token || exit 1
am start -a android.intent.action.VIEW -d "$ADMIN_URL"
