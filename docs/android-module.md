# Android 模块

[← 返回 README](../README.md)

## 设备要求

- KernelSU
- Android 6.0 / API 23 及以上
- arm64-v8a

模块不修改 `system/`，不需要 metamodule。网关固定监听
`127.0.0.1:7863`，不直接暴露到局域网。

## 目录

```text
/data/adb/modules/wb2api/
├── module.prop
├── config.json
├── keys.json
├── auths/
├── data/
├── logs/
├── run/
├── tmp/
├── bin/
│   ├── wbapi
│   ├── wb2api-server
│   └── wb2api-login
├── customize.sh
├── service.sh
├── action.sh
└── uninstall.sh
```

所有持久数据和日志都在模块目录内。安装升级时会保留 `config.json`、
`keys.json`、`auths/`、`data/` 和 `logs/`。卸载模块会删除全部数据。

`bin/wbapi` 是 Android 专用 CLI（源码在 `cmd/wbapi-android`），会拒绝配置中
指向模块目录外的 `auth_dir`、`keys_file` 和 `state_file`。桌面版 CLI 位于
`internal/wbapicli`，不参与模块构建。

## 首次使用

重启后可以点击模块的“操作”按钮。`action.sh` 会先启动网关，再打开
`http://127.0.0.1:7863/admin/` 并显示管理 token。

也可以使用绝对路径运行 CLI：

```bash
/data/adb/modules/wb2api/bin/wbapi status
/data/adb/modules/wb2api/bin/wbapi login --realm=cn
/data/adb/modules/wb2api/bin/wbapi panel --show-token
```

首次安装会生成随机 `api_key`，同时写入 `keys.json`。管理后台默认使用
`keys.json` 的第一个 key。

## CLI

```text
wbapi status [--json]
wbapi accounts
wbapi quota [test [模型]]
wbapi acct list|disable|enable|revive
wbapi task checkin|activity|travel|keepalive|school|cat [--wait]
wbapi key list|new|revoke
wbapi models [--realm cn|global]
wbapi stats [--reset]
wbapi panel [--show-token]
wbapi config
wbapi log [N]
wbapi start|stop|restart
wbapi doctor
wbapi login [--realm=cn|global]
```

CLI 不依赖 Python、systemd、journalctl 或 Docker。服务使用模块内的 PID
文件管理，日志写入 `logs/gateway.log`。

## 自动任务

安卓版不启动自动调度，`service.sh` 会设置：

```sh
WB2A_SCHEDULER=0
```

签到、旅行、活跃上报、保活、开学季和夜猫子都不会自动运行，但可以在管理
面板或 CLI 里按需触发：

```bash
$WBAPI task checkin
$WBAPI task school
$WBAPI task cat
```

所有任务都由 `internal/tasks` 的 Go 实现执行，不依赖 Python。若确实需要自动
调度，可以删掉 `service.sh` 中的 `WB2A_SCHEDULER=0`，再在 `config.json` 的
`schedule` 段按任务开启。

## 构建

构建脚本默认使用 `/home/yule/opt/android-ndk-r27c`，也可以通过
`ANDROID_NDK_HOME` 指定：

```bash
ANDROID_NDK_HOME=/path/to/android-ndk ./scripts/build_android.sh
```

构建使用 NDK clang、`CGO_ENABLED=1` 和 `netcgo`。不能在 Android 上依赖
`CGO_ENABLED=0` 的纯 Go DNS 解析。

```bash
make build
make test
```

`make build` 生成：

```text
build/stage/
build/wb2api-v0.2.0-beta.1-arm64-v8a.zip
```

## 验证

```bash
go test ./...
go vet ./...
shellcheck module/*.sh scripts/build_android.sh scripts/test_customize.sh
shfmt -i 4 -d module/*.sh scripts/build_android.sh scripts/test_customize.sh
python3 ~/.agents/skills/magisk-kernelsu-module/scripts/validate_module.py build/stage
```

发布前仍需在真机验证安装、重启、Action 按钮、管理后台、CLI 登录、
OpenAI 接口、模块升级和卸载。
