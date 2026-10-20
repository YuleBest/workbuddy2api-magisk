# WorkBuddy2API KernelSU 模块

在 Android 手机上以 KernelSU 模块运行 WorkBuddy2API。网关只监听
`127.0.0.1:7863`，管理后台随服务端二进制一起分发。

上游：Sliverkiss/workbuddy2api -> [YuleBest/workbuddy2api](https://github.com/YuleBest/workbuddy2api) -> [YuleBest/workbuddy2api-magisk](https://github.com/YuleBest/workbuddy2api)

![WorkBuddy2API 管理后台，展示总览、账号池和模型页面](docs/images/admin-panel.jpg)

## 支持范围

- KernelSU
- Android 6.0 / API 23 及以上
- arm64-v8a
- 自动定时任务关闭
- 不包含 Python 的 school/cat 任务
- 不包含 Cloudflare Tunnel

## 构建与安装

从 [Releases](https://github.com/YuleBest/workbuddy2api-magisk/releases)
下载 `wb2api-*-arm64-v8a.zip`，刷入 KernelSU 后重启。

本地开发构建需要 Go、Android NDK r27c、`zip`、`shellcheck`、`shfmt`。

```bash
make build
```

产物位于 `build/`，同时生成 `.sha256` 校验文件。

## 使用

点击模块的“操作”按钮会启动网关并打开管理后台。管理 token 会显示在
KernelSU 的操作输出中。

命令行工具位于模块内：

```bash
WBAPI=/data/adb/modules/wb2api/bin/wbapi
$WBAPI status
$WBAPI panel --show-token
```

配置、凭证、状态、日志和临时文件全部位于
`/data/adb/modules/wb2api`。模块更新会迁移这些文件，卸载会随模块目录删除。

## 账号管理

在手机的 root shell 中运行以下命令，浏览器完成授权后会新增一个账号：

```bash
WBAPI=/data/adb/modules/wb2api/bin/wbapi
$WBAPI login --realm=cn
$WBAPI login --realm=global
```

账号凭证保存在模块的 `auths/`。已有部署迁移时，可以复制该目录；如需保留
积分和历史状态，同时复制原部署的 `data/state.json`。

管理后台的“账号池”页面可以查看积分、冷却和限额，并执行停用、恢复或复活。
也可以使用绝对路径运行 CLI：

```bash
$WBAPI accounts
$WBAPI acct disable <uid> [原因]
$WBAPI acct enable <uid>
$WBAPI acct revive <uid>
$WBAPI task checkin
```

Android 版不运行自动任务，积分来自上一次签到或余额查询。新账号登录后，
建议执行一次 `$WBAPI task checkin` 刷新余额。

详细说明见 [Android 模块文档](docs/android-module.md)，上游服务能力见
[功能文档](docs/features.md) 与 [管理文档](docs/admin.md)。

本项目使用 MIT 许可。
