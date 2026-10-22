# 管理后台与 CLI

[← 返回 README](../README.md)

## Web 管理面板

管理面板已经嵌入网关二进制，不需要单独部署前端服务或数据库。

先在 `config.json` 中启用：

```json
{
  "api_key": "replace-with-a-random-key",
  "admin": {
    "enabled": true
  }
}
```

重启网关后访问：

```text
http://127.0.0.1:7863/admin/
```

登录 token 的优先级为：

1. `admin.token`
2. `keys.json` 中的第一个 key
3. `api_key`

### 页面功能

| 页面 | 功能 |
| --- | --- |
| 总览 | 运行状态、账号分布、请求量、Token 统计和定时任务 |
| 账号池 | 积分、冷却、停用状态、限额、成本、在途请求；执行停用、恢复、复活和清冷却 |
| 调用密钥 | 查看掩码、创建和吊销调用 key |
| 请求日志 | 查看最近 500 条请求（含实际积分消耗与模型积分倍率），按模型、昵称和状态筛选 |
| 模型 | 查看模型、域、倍率、能力、上下文和推理档位 |
| 模型对话 | 直接测试模型，请求会经过完整网关链路 |
| 设置 | 查看脱敏配置和运行信息，调整主题与刷新频率 |

面板只做观测和运维动作，不直接修改配置文件。设置页是只读的。

关闭管理面：

```json
{
  "admin": {
    "enabled": false
  }
}
```

关闭后，`/admin/`、`/api/admin/*` 和 `/admin/accounts/*` 都不会注册。

## 多调用密钥

`keys.json` 保存多个调用 key，格式如下：

```json
{
  "keys": [
    {
      "key": "sk-...",
      "note": "笔记本",
      "added": "2026-10-20"
    }
  ]
}
```

规则：

- 文件路径由 `keys_file` 指定，默认 `keys.json`。
- 文件权限建议为 `0600`。
- 文件不存在或为空时回落到 `api_key`。
- key 文件变化后自动热加载，不需要重启。
- JSON 损坏时保留上一份可用 key，并在日志中报错。

常用命令：

```bash
./wbapi key list
./wbapi key new "笔记本"
./wbapi key revoke sk-前缀
```

## wbapi CLI

`wbapi` 是 Go 可执行文件，会自动读取运行目录中的 `config.json`。

Android 模块内也提供同名 `wbapi`，命令面基本一致；差异是不管理 cloudflared
隧道，`start|stop|restart` 使用模块 PID 文件而不是 systemd，`school|cat`
可以手动触发但默认不自动调度。详见 [Android 模块](android-module.md)。

先查看帮助：

```bash
./wbapi help
./wbapi help acct
./wbapi help task checkin
```

所有命令都支持 `-h` 和 `--help`：

```bash
./wbapi task checkin --help
```

### 状态与诊断

| 命令 | 作用 |
| --- | --- |
| `wbapi status` | 查看网关、隧道、健康状态和账号池概况，默认命令 |
| `wbapi doctor` | 检查服务、二进制、权限、配置和运行态 |
| `wbapi accounts` | 查看完整账号池 |
| `wbapi quota` | 查看冷却与模型限额 |
| `wbapi quota test [模型...]` | 发送最小请求测试模型可用性 |
| `wbapi models [--realm cn\|global] [--more]` | 查看模型信息 |
| `wbapi stats [--reset]` | 查看或清零请求统计 |
| `wbapi log [行数]` | 查看网关日志 |
| `wbapi config` | 查看脱敏配置 |
| `wbapi panel` | 显示管理面板地址和登录提示 |

### 账号与任务

| 命令 | 作用 |
| --- | --- |
| `wbapi acct list` | 列出账号，等价于 `wbapi accounts` |
| `wbapi acct disable <uid\|前缀> [原因]` | 手动停用账号 |
| `wbapi acct enable <uid\|前缀>` | 清除手动停用 |
| `wbapi acct revive <uid\|前缀>` | 清除系统自动禁用 |
| `wbapi task checkin` | 执行签到，默认等待逐号结果 |
| `wbapi task activity\|travel\|keepalive\|school\|cat` | 触发其他任务 |

手动停用与系统禁用相互独立。两者都清除后，账号才回到选号池。

### 服务管理

| 命令 | 作用 |
| --- | --- |
| `wbapi start` | 启动网关和隧道 |
| `wbapi stop` | 停止网关和隧道 |
| `wbapi restart` | 重启网关和隧道 |
| `wbapi tunnel start\|stop\|restart\|status` | 只操作隧道 |

Linux 使用 systemd，服务名为 `wbapi-gateway` 和 `wbapi-tunnel`。Windows 使用同名计划任务。
macOS 使用 Release 包时，可以直接从终端启动，或自行创建 LaunchAgent。

## 环境变量

| 变量 | 作用 |
| --- | --- |
| `WB2A_HOME` | 运行根目录，默认可执行文件所在目录 |
| `WB2A_CONFIG` | 配置文件路径 |
| `WB2A_GATEWAY` | 直接指定网关地址 |
| `WB2A_PUBLIC_HOST` | 面板和状态显示的公网域名 |

更多配置说明见[配置文档](config.md)。
