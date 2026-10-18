# 管理后台与运维 CLI

[← 返回 README](../README.md)

> Android 模块使用原生 Go CLI，入口是
> `/data/adb/modules/wb2api/bin/wbapi`。下文 Linux/Windows 的
> systemd、journalctl、计划任务和 Cloudflare Tunnel 说明不适用于安卓版，
> 详见 [Android 模块](android-module.md)。

## Web 管理后台

网关内置一个 Vue 3 管理面板，和二进制一起分发，不需要额外部署前端或数据库。

```bash
# 浏览器打开（把 host 换成你的部署地址；容器部署映射了端口就是宿主机地址）
http://localhost:7863/admin/
```

**登录**：填一个调用 key（`keys.json` 里的任意一个），或 `admin.token` 单独配置的管理 token。
token 来源优先级：`admin.token` → `keys.json` 首个 key → `config.json` 的 `api_key`；
三者都为空时管理接口不鉴权（与网关"`api_key` 留空 = 不鉴权"同一语义，仅限内网使用）。

| 页面 | 能看到什么 | 能做什么 |
|---|---|---|
| 总览 | 是否可接活、账号池分布、请求量与首字延迟、定时任务排程 | 立即执行某类定时任务；签到会同步等待并列出每个账号的结果与签到后余额 |
| 账号池 | 每号积分余量、冷却倒计时与原因、模型级限额台账、连败降权、在途请求、凭证过期时间、按模型的实测成本、域（国内 / 国际） | 停用 / 恢复（手动位）、复活（系统自动禁用位）、清冷却 |
| 调用密钥 | 已签发的 key（掩码）、备注、添加日期 | 新建（完整值只显示一次）、吊销 |
| 请求日志 | 进程内最近 500 条请求：模型、昵称（uid8）、状态码、首字延迟、token、速率 | 按模型 / 账号 / 状态码筛选，只看失败 |
| 模型 | 上游动态模型表：域前缀、上下文长度、推理档位、倍率 | 搜索、按表头排序、复制模型 ID、展开完整字段 |
| 模型对话 | 选模型直接开聊，流式逐字输出，助手回复按 markdown 渲染（代码块 / 列表 / 表格） | 发送 / 停止；对话走后端数据面，会进请求日志与统计 |
| 设置 | 生效配置（密钥类只显示"是否已配置"）、运行信息 | 调整页面刷新间隔与主题、退出登录 |

**前置条件**：`config.json` 里 `"admin": {"enabled": true}`（默认关闭，与 `cmd/acct` / `acct.sh` 同一开关）。
面板的动作与上游 `/admin/accounts/{uid}/{disable,enable,revive}` 端点同语义、同状态位——
`disable/enable` 管手动停用位，`revive` 管系统自动禁用位，两者独立。

**边界**：面板只做观测与少量运维动作，配置本身是只读的——改配置仍要编辑 `config.json` 并重启服务，
避免在浏览器里改坏关键参数。请求日志来自进程内存环形缓冲，重启即清空；要长期留档看服务日志。
「模型对话」会被网关注入 system prompt（跟 `prompt.mode` 走），并占用账号池的在途租约，
池子满时可能收到 503——这是真实链路语义，不是面板缺陷。

**关掉面板**：`config.json` 里设 `"admin": {"enabled": false}`，`/admin`、`/api/admin/*` 与
`/admin/accounts/*` 端点都不再注册（`acct.sh` 同样不可用）。

## 调用密钥（多 key）

上游只有 `config.json` 里的一把 `api_key`，本 fork 增加了多 key 存储，方便给不同客户端各发一把、单独吊销。

```json
// keys.json（默认路径，可被 config 的 keys_file 改写；文件权限 0600，已被 .gitignore 排除）
{
  "keys": [
    { "key": "sk-…", "note": "笔记本", "added": "2026-09-22T10:00:00+08:00" }
  ]
}
```

- **热重载**：文件内容一变（按 mtime 判断）立刻生效，新增 / 吊销**不用重启网关**；
- **回落**：文件不存在或为空时用 `config.json` 的 `api_key`，两者都空 = 不鉴权（仅限内网）；
- **文件损坏不致命**：JSON 解析失败会保留上一份可用 key 并打日志，不会把自己锁在门外；
- **管理方式**：面板「调用密钥」页，或 `wbapi key list|new|revoke`（直接读写文件，网关自动热加载）。

## 运维 CLI（wbapi）

仓库根的 `wbapi` 是一个 Python 3 脚本，把日常运维收成一个命令，自动读 `config.json` 拿网关地址与 key：

| 命令 | 作用 |
|---|---|
| `wbapi status` | 网关 / 隧道状态 + `/healthz` 探活 + 池子计数（默认动作，别名 `check`） |
| `wbapi doctor` | 自检：服务入口与二进制、`auths/` 属主与可读性、`keys.json` 权限、管理面前置、运行态；有问题时非零退出 |
| `wbapi accounts` | 账号池明细：域 / 积分 / 冷却 / 双位停用 / 限额台账 / 实测成本 |
| `wbapi quota [test [模型]]` | 冷却与限额详情；`test` 用 `max_tokens=16` 实测模型可用性 |
| `wbapi acct list\|disable <uid> [原因]\|enable <uid>\|revive <uid>` | 账号运维动作（与面板、`acct.sh` 同一批端点） |
| `wbapi task <checkin\|activity\|travel\|keepalive\|school\|cat> [--wait]` | 手动触发定时任务；签到默认同步等待并打印逐号回执 |
| `wbapi key list [--show]\|new [备注]\|revoke <前缀>` | 调用密钥的查看 / 新建 / 吊销 |
| `wbapi models [--realm cn\|global] [--more]` | 模型表格：名 / ID / 域 / 倍率 / 能力 / 上下文 / 输出；`--more` 打完整字段 |
| `wbapi stats [--reset]` | 请求统计（`/v1/stats`；`--reset` 清零累计） |
| `wbapi panel` | 管理后台地址与登录提示 |
| `wbapi config` | 生效配置（脱敏） |
| `wbapi log [N]` | 网关最近 N 行日志（默认 30） |
| `wbapi start\|stop\|restart` | 网关 + 隧道（Linux 走 systemd，Windows 走计划任务） |
| `wbapi tunnel [start\|stop\|restart\|status]` | 只操作隧道 |

环境变量：`WB2A_HOME`（项目根，默认脚本所在目录）、`WB2A_CONFIG`（配置文件，默认 `$WB2A_HOME/config.json`）、`WB2A_GATEWAY`（直接指定网关地址）、`WB2A_PUBLIC_HOST`（公网域名，仅用于状态显示）。

Linux 下服务名固定为 `wbapi-gateway` / `wbapi-tunnel`；Windows 下对应同名计划任务与 `server.exe` / `cloudflared.exe` 进程。

## 第三方面板

内置面板之外，还有符合「面板与网关解耦」理念的社区项目（独立维护，与上游同步）：

- [workbuddy2api-gui](https://github.com/287775856/workbuddy2api-gui) — 账号池状态可视化面板
- [workbuddy-manager](https://github.com/ithtelab/workbuddy-manager) — 账号管理工具
