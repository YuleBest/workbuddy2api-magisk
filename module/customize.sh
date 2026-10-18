#!/system/bin/sh
# shellcheck shell=ash

if [ "$KSU" != "true" ]; then
    abort "wb2api 仅支持 KernelSU"
fi
if [ "$ARCH" != "arm64" ]; then
    abort "wb2api 仅支持 arm64-v8a，当前: $ARCH"
fi
if [ "$API" -lt 23 ]; then
    abort "wb2api 需要 Android 6.0/API 23 或更高版本"
fi

OLD_MODDIR="/data/adb/modules/wb2api"

if [ -x "$OLD_MODDIR/bin/wbapi" ]; then
    "$OLD_MODDIR/bin/wbapi" stop --quiet >/dev/null 2>&1 || true
fi

preserve_item() {
    item="$1"
    if [ -e "$OLD_MODDIR/$item" ]; then
        rm -rf "${MODPATH:?}/$item"
        cp -a "$OLD_MODDIR/$item" "${MODPATH:?}/$item"
    fi
}

for item in config.json keys.json auths data logs; do
    preserve_item "$item"
done

mkdir -p "$MODPATH/auths" "$MODPATH/data" "$MODPATH/logs" "$MODPATH/run" "$MODPATH/tmp"

if [ ! -f "$MODPATH/config.json" ]; then
    random_hex="$(od -An -N16 -tx1 /dev/urandom | tr -d ' \n')"
    if [ "${#random_hex}" -ne 32 ]; then
        abort "生成随机 api_key 失败"
    fi
    api_key="sk-$random_hex"
    sed "s/__API_KEY__/$api_key/g" "$MODPATH/config.android.json" >"$MODPATH/config.json"
    cat >"$MODPATH/keys.json" <<EOF
{
  "keys": [
    {
      "key": "$api_key",
      "note": "KernelSU 安装生成",
      "added": "$(date +%Y-%m-%d)"
    }
  ]
}
EOF
fi

rm -f "$MODPATH/config.android.json"

if [ ! -f "$MODPATH/keys.json" ] && [ -f "$MODPATH/config.json" ]; then
    ui_print "! keys.json 缺失，管理后台将回落到 config.json 的 api_key"
fi

set_perm "$MODPATH" 0 0 0755
set_perm "$MODPATH/bin" 0 0 0755
set_perm "$MODPATH/customize.sh" 0 0 0755
set_perm "$MODPATH/service.sh" 0 0 0755
set_perm "$MODPATH/action.sh" 0 0 0755
set_perm "$MODPATH/uninstall.sh" 0 0 0755
set_perm "$MODPATH/bin/wbapi" 0 0 0755
set_perm "$MODPATH/bin/wb2api-server" 0 0 0755
set_perm "$MODPATH/bin/wb2api-login" 0 0 0755
set_perm "$MODPATH/auths" 0 0 0700
set_perm "$MODPATH/data" 0 0 0700
set_perm "$MODPATH/logs" 0 0 0700
set_perm "$MODPATH/run" 0 0 0700
set_perm "$MODPATH/tmp" 0 0 0700
set_perm "$MODPATH/config.json" 0 0 0600
set_perm "$MODPATH/keys.json" 0 0 0600

ui_print "********************************"
ui_print " WorkBuddy2API 已安装"
ui_print " 管理后台: http://127.0.0.1:7863/admin/"
ui_print " 管理命令: /data/adb/modules/wb2api/bin/wbapi"
ui_print " 查看 token: /data/adb/modules/wb2api/bin/wbapi panel --show-token"
ui_print " 自动定时任务: 已禁用"
ui_print " 重启设备后生效"
ui_print "********************************"
