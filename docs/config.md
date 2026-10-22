# 配置

[← 返回 README](../README.md)

网关使用一个 JSON 配置文件，默认路径为运行目录下的 `config.json`，可以通过 `-config` 指定其他路径。

完整字段和默认值以 [`config.example.json`](../config.example.json) 为准。本文只说明常用配置分组和生效规则。

## 最小配置

```json
{
  "listen": "127.0.0.1:7863",
  "api_key": "replace-with-a-random-key"
}
```

`api_key` 留空表示网关不鉴权，只适合完全隔离的本机环境。

## 基础配置

| 键 | 默认值 | 说明 |
| --- | --- | --- |
| `listen` | `:7863` | HTTP 监听地址，建议本机部署使用 `127.0.0.1:7863` |
| `api_key` | 空 | 默认调用密钥，留空表示不鉴权 |
| `keys_file` | `keys.json` | 多 key 文件，文件不存在或为空时回落到 `api_key` |
| `auth_dir` | `auths` | 账号凭证目录 |
| `state_file` | `data/state.json` | 账号池状态文件 |

相对路径均从网关工作目录解析。生产环境中建议把工作目录固定为运行目录，不要指向源码目录。

## 管理面板

| 键 | 默认值 | 说明 |
| --- | --- | --- |
| `admin.enabled` | `false` | 启用面板与 `/admin/*`、`/api/admin/*` 端点 |
| `admin.token` | 空 | 管理面专用 token，为空时回落到首个调用 key，再回落到 `api_key` |

开启管理面时 `api_key` 不能为空，否则网关会拒绝启动。

## 提示词与请求兼容

| 键 | 默认值 | 说明 |
| --- | --- | --- |
| `prompt.mode` | `passthrough` | `passthrough`、`custom` 或 `append` |
| `prompt.file` | 空 | 自定义提示词文件，空表示使用内置提示词 |
| `features.sanitize_blacklist_fingerprints` | `true` | 清洗出站请求体中的指纹字段 |
| `upstream.*` | 见示例 | 超时、客户端标识和设备 token |

`custom` 会替换客户端的 system/developer 提示词。`append` 会在客户端提示词后追加网关提示词。只想保持原协议行为时使用 `passthrough`。

## 账号池与冷却

| 键 | 说明 |
| --- | --- |
| `pool.max_in_flight` | 单账号最大在途请求数 |
| `pool.max_in_flight_global` | 国际版域单账号的最大在途请求数 |
| `pool.breaker_threshold` | 连续失败触发熔断的阈值 |
| `pool.expiring_soon` | 快过期积分的优先窗口 |
| `pool.cost_explore_interval` | 未知成本账号的探索间隔，设为 `"0"` 可关闭 |
| `cooldown.*` | 429 软冷却的基数和上限 |
| `session_sticky.*` | 会话粘性开关、TTL 和清理间隔 |

修改这些配置会影响选号、冷却和并发行为。调整前先运行 `wbapi accounts` 和 `wbapi quota` 观察当前状态。

## 定时任务

`schedule.*` 控制六类任务的执行时点和开关：

- `checkin_hours`：签到
- `activity_hours`：活跃地图
- `travel_hours`：猫猫旅行
- `keepalive_hours`：token 保活
- `school_hours`：开学季任务
- `cat_hours`：夜猫子任务

每类任务都有独立的 `*_enabled` 开关。手动执行使用 `wbapi task <任务>`。

Android 模块的配置位于 `/data/adb/modules/wb2api/config.json`，安装时由
`module/config.android.json` 生成；`auth_dir`、`keys_file`、`state_file` 必须
留在模块目录内。`service.sh` 设置 `WB2A_SCHEDULER=0`，所以即使 `*_enabled`
为 true 也不会自动运行，手动任务不受影响。

## 双域与存储

| 键 | 默认值 | 说明 |
| --- | --- | --- |
| `global.enabled` | `true` | 是否允许国际版账号和 `global:` 模型路由 |
| `upstash.url` | 空 | 可选的 Upstash Redis 地址 |
| `upstash.token` | 空 | Upstash Redis token |

Redis 用于镜像池状态和会话绑定。Redis 不可用时，网关仍可使用本地 `state.json` 运行。

## 环境变量

| 变量 | 作用 |
| --- | --- |
| `WB2A_ADMIN_TOKEN` | 覆盖 `admin.token` |
| `WB2A_HOME` | `wbapi` CLI 使用的运行根目录，默认可执行文件所在目录 |
| `WB2A_CONFIG` | 覆盖 `wbapi` CLI 使用的配置文件路径 |
| `WB2A_GATEWAY` | 覆盖网关地址，例如 `http://127.0.0.1:7863` |
| `WB2A_PUBLIC_HOST` | 面板和状态输出中显示的公网域名 |

## 生效规则

- 大部分配置需要重启网关后生效。
- 新增或更新 `auths/` 凭证会自动加载，不需要重启。
- 修改 `keys.json` 会自动热加载，不需要重启。
- 管理面板中的设置页只读，不会在线修改配置文件。

修改配置后运行：

```bash
./wbapi doctor
./wbapi status
```
