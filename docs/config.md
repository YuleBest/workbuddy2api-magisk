# 配置

[← 返回 README](../README.md)

配置是单个 JSON 文件（默认 `config.json`，由 `-config` 指定）。**权威定义看
[`config.example.json`](../config.example.json)** 与 `cmd/server/config.go` 的结构体注释，
本文只列要点与本 fork 新增项。

## 关键配置段

| 键 | 说明 |
|---|---|
| `listen` | 监听地址，默认 `:7863`；建议本机部署设为 `127.0.0.1:7863` |
| `api_key` | 调用 key；**留空 = 不鉴权**，公网部署务必设置 |
| `keys_file` | 多 key 文件路径，默认 `keys.json`；文件缺失 / 为空时回落 `api_key`（见 [管理后台与 CLI](admin.md)） |
| `auth_dir` / `state_file` | 凭证目录与池状态落盘路径，默认 `./auths` / `./data/state.json`，相对工作目录解析 |
| `admin.enabled` | 管理面开关（上游端点 + 面板 + CLI 动作），**默认 false**；开启时要求 `api_key` 非空（fail-fast） |
| `admin.token` | 管理面专用 Bearer token，空 = 回落 `keys.json` 首个 key → `api_key` |
| `prompt.mode` / `prompt.file` | 提示词模式 `passthrough`（缺省）/ `custom` / `append`，以及自定义提示词文件 |
| `pool.*` | 在途上限、熔断阈值与退避、连败降权、闲置补偿、快过期积分窗口、成本探索间隔 |
| `cooldown.*` | 429 软冷却基数与封顶 |
| `session_sticky.*` | 会话粘性开关、TTL、GC 间隔 |
| `schedule.*` | 六类定时任务的时点数组与独立开关（`checkin_hours` / `travel_hours` / … / `*_enabled`） |
| `upstream.*` | 超时分层（`timeout_seconds` / `header_timeout_seconds` / `idle_timeout_seconds`）、客户端标识、设备 token |
| `global.enabled` | 是否启用国际版域；`false` 可锁死纯 CN 部署 |
| `features.sanitize_blacklist_fingerprints` | 出站请求体指纹脱敏开关 |
| `upstash.url` / `upstash.token` | 可选的池状态 Redis 镜像 |

## 环境变量

| 变量 | 作用 |
|---|---|
| `WB2A_ADMIN_TOKEN` | 设置 `admin.token`，优先级高于配置文件 |
| `WB2A_HOME` | `wbapi` CLI 的项目根目录（默认脚本所在目录） |
| `WB2A_CONFIG` | `wbapi` CLI 的配置文件路径（默认 `$WB2A_HOME/config.json`） |
| `WB2A_GATEWAY` | 直接指定网关地址，覆盖配置里的 `listen` |
| `WB2A_PUBLIC_HOST` | 公网域名，仅用于 `wbapi` 的状态显示 |

## 注意

- 改配置需要**重启服务**才生效（管理面板里的配置页是只读的，避免在浏览器里改坏关键参数）。
- `admin.enabled=true` 且 `api_key` 为空时网关**启动即报错退出**，这是有意的：管理端点不能裸奔。
- 升级上游后先看 `config.example.json` 的默认值变化（例如 `server.max_body_mb` 已被移除）。
