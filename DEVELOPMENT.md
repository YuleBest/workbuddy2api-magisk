# 开发说明

给仓库维护者与贡献者。部署与使用看 `README.md`。

## Android 模块

本仓库的 Android 交付物是 KernelSU 模块。模块源码在 `module/`，构建脚本
在 `scripts/build_android.sh`，产物写入 `build/`。

```bash
make build
make test
```

Android 构建使用 NDK r27c 和目标 API 23：

```text
GOOS=android
GOARCH=arm64
CGO_ENABLED=1
-tags netcgo
```

必须使用 cgo resolver。Android 没有常规 `/etc/resolv.conf`，
`CGO_ENABLED=0` 的纯 Go DNS 会不可靠。Android 下 Go 默认使用 UTC，
因此服务端和 CLI 都调用 `internal/timezone.Init()` 固定为
`Asia/Shanghai`。

Android 运行时文件全部位于 `/data/adb/modules/wb2api`。CLI 通过
自身路径推导模块根目录，并拒绝配置中指向模块目录外的 `auth_dir`、
`keys_file` 和 `state_file`。

`cmd/wbapi` 是 Android 管理 CLI，不依赖 Python。进程启动使用
`Setsid`，PID 文件位于 `run/wbapi.pid`，日志位于
`logs/gateway.log`。`service.sh` 只负责调用 `wbapi start --quiet`
并立即返回。

自动调度通过 `WB2A_SCHEDULER=0` 关闭。`school` 和 `cat` 的 Python
入口通过 `WB2A_PYTHON_TASKS=0` 拒绝。

## 分支与上游

本仓库是 [`Sliverkiss/workbuddy2api`](https://github.com/Sliverkiss/workbuddy2api) 的 fork，
`master` 承载本 fork 的全部改动（内置管理后台、多 key、`wbapi` CLI、Windows 部署适配等）。
上游同步用 rebase，保持本地提交线性：

```bash
git remote add upstream https://github.com/Sliverkiss/workbuddy2api.git   # 只需一次
git fetch upstream && git rebase upstream/master
```

本 fork 的改动都是**增量**的：不改上游配置键语义、不删上游端点，所以可以直接替换上游二进制。

## 目录结构

```
cmd/server          网关主程序（配置加载、pool/scheduler 组装、HTTP 服务）
cmd/wbapi           Android/KernelSU 管理 CLI
cmd/login           设备授权登录 CLI
cmd/signin          单次签到 CLI
cmd/credit          积分查询 CLI
cmd/activity        活跃上报一次性触发器
cmd/trial           国际版注册 / trial 加油包 CLI
cmd/acct            账号停用 / 恢复 / 复活 CLI（走网关管理端点）
cmd/stats           统计 CLI
internal/admin      管理后台：观测 API + 运维动作 + 内嵌 SPA（dist 由 web/ 构建产出）
internal/server     数据面：OpenAI 兼容 handler、鉴权、多 key 存储、请求日志
internal/auth       凭证解析与原子写回
internal/config     跨命令共享的配置段（schedule）与默认值
internal/pool       账号池：选号 / 冷却 / 熔断 / 在途租约 / 状态持久化
internal/scheduler  定时任务：签到 / 活跃上报 / 猫猫旅行 / 保活 / 开学季 / 夜猫子
internal/session    会话粘性路由
internal/upstream   上游客户端：SSE 流、请求体改写管线（含 ZCode 归一化）、错误分类
internal/prompt     内置提示词
internal/logfmt     日志行格式化
internal/redisstore 池状态 Redis 镜像
internal/timezone   Android 时区初始化
web                 Vue 3 + Vite + TS 管理面板源码
module              KernelSU 模块脚本、配置模板与元数据
wbapi               运维 CLI（Python 3，仓库根）
```

## 环境要求

- Go ≥ 1.22（`go.mod` 声明 1.22.5）
- Node ≥ 20 + pnpm ≥ 9（只改后端时不需要）
- Python 3（跑 `wbapi` 时才需要）
- 一个已注册的 CodeBuddy 账号（跑真实请求时才需要）

## 常用命令

```bash
# 后端
go build ./...
go vet ./...
go test ./...            # 完整测试套件
go test -race ./internal/admin/ ./internal/pool/ ./internal/server/

# 前端（web/ 目录）
pnpm install
pnpm dev                 # Vite 开发服务器，/api/admin 代理到 127.0.0.1:7863
pnpm build               # 类型检查 + 构建，产物落 internal/admin/dist（被 go:embed 拾取）
pnpm build:only          # 跳过类型检查的快速构建
```

## 管理后台的构建链路

```
web/src/*.vue ──vite build──▶ internal/admin/dist/ ──go:embed──▶ 二进制
```

- `internal/admin/dist` 是构建产物，**提交进版本库**：这样只改后端的人（以及 Dockerfile 的
  `go build`）不需要 Node 就能编译。**前端产物和后端代码必须一起提交**，否则二进制里是旧页面。
- 改完前端必须重新 `pnpm build` 再 `go build`。
- `internal/admin/webui.go` 负责静态资源与 SPA 回退：命中文件按扩展名给 Content-Type，
  `assets/` 下（内容哈希文件名）长缓存，其余 `no-cache`，未知路径回退 `index.html` 交给前端路由。
- 前端路由是 history 模式，base 固定 `/admin/`；Go 侧的路由前缀与 `vite.config.ts` 的 `base`
  必须一致，换前缀要同时改。
- 前端路由表在 `web/src/router.ts`，未登录只有 `/login` 可达，鉴权状态来自 `GET /api/admin/session`。

## 两套管理入口的关系

网关有两组管理端点，语义同源、客户端不同，**不要各写一套状态机**：

| 入口 | 路径 | 客户端 |
|---|---|---|
| 上游运维端点 | `POST /admin/accounts/{uid}/{disable,enable,revive}` | `acct.sh` / `cmd/acct` / `wbapi acct` |
| Web 后台 API | `GET/POST /api/admin/*` | 浏览器面板（`web/`） |

- 两边都走 `pool` 的同一批方法：`SetManualDisabled`（手动停用位）/ `ReviveDisabled`（系统自动禁用位）/
  `ManualDisabledState`（读双位）/ `ClearCooldown`（清软冷却与降权，不动熔断与积分）。
  **手动停用与系统自动禁用是两个独立状态位**：`enable` 只解手动位，
  仍被系统禁用的号需要 `revive` 才回选号池。
- 上游端点的模式表在 `internal/server/handler.go` 的 `adminRoutes`，经 `server.AdminRoutePatterns()`
  暴露给 `cmd/server`：后台前端挂在 `/admin/` 前缀下，必须先把这些模式转回数据面，
  否则会被 SPA 吃掉（`acct.sh` 会 404）。**上游新增管理端点时同步这张表即可，不用改 main.go。**
- 开关是同一个 `config.admin.enabled`（默认 false），且要求 `api_key` 非空（fail-fast）。

## 管理 API 约定

- 全部挂在 `/api/admin/*`，Bearer token 鉴权（常量时间比较）。token 取 `admin.token` →
  `keys.json` 首个 key → `api_key`；三者都空时不鉴权。
- 响应一律 JSON；错误形如 `{"error":{"code":"...","message":"..."}}`。
- 只读接口：`session` / `overview` / `accounts` / `keys`(GET) / `logs` / `models` / `config`。
- 动作接口：账号 `disable|enable|revive|clear-cooldown`、密钥 `POST` / `DELETE`、
  `tasks/{checkin|activity|travel|keepalive|school|cat}`。
- `tasks/*` 默认异步（202，结果写服务日志）；`tasks/checkin?wait=1` 同步等待并回传逐号回执
  （签到是秒级短任务，公网隧道等得起，上限 80s；旅行/活跃上报按账号限速，同步会撑到分钟级，故不支持 wait）。
- `POST /api/admin/v1/chat/completions` 是面板「模型对话」的入口，走 OpenAI 兼容面而非 REST
  （`/api/admin/models` 返回 `{models:[...]}`，混用会让 SDK 误判）。它把请求体的 `Authorization`
  换成 `keys.First()`、透传客户端 IP 头，再把**原始 ResponseWriter** 交给数据面 handler——SSE 直写浏览器，
  选号 / 粘性 / 冷却 / reqlog / 统计零复制照走，没有可信旁路。请求体上限 4MB，`Chat == nil` 时 503。
- 动作接口只调用 `pool` / `scheduler` / `server.KeyStore` 的既有入口，不直接改内部状态；
  新增动作时优先复用这些入口，别在 admin 包里另写一套状态机。
- 脱敏：`config` 接口不回传任何密钥明文（只给 `*_set` 布尔）；`accounts` 里 uid 全量返回给管理端
  （运维需要区分账号），但**日志**里只打前 8 位。

## 多 key 存储（internal/server/keystore.go）

- 文件格式：`{"keys":[{"key","note","added"}]}`（`note` 为首选备注字段，兼容旧的 `comment`）。
- 路径取 `config.keys_file`（默认 `keys.json`，相对工作目录）；文件不存在或为空时回落 `config.api_key`，
  两者都空 = 不鉴权。
- **热重载**：每次 `Valid` / `AuthRequired` / `Count` / `First` / `List` 都按 mtime 判断是否重读，
  所以增删 key 不需要重启；解析失败保留上一份可用 key（不把自己锁在门外）。
- `Valid` 对所有 key 做常量时间比较且不提前返回；`Add` 生成 `sk-<32 hex>` 并以 tmp+rename 原子写入（0600）；
  `Remove` 要求前缀唯一匹配；`List` 只返回掩码 + 前缀 + 备注。
- 面板「调用密钥」页与 `wbapi key` 都直接操作这个文件，不经过内存状态。

## 上游请求体归一化（internal/upstream/zcode.go）

`payload.go` 的 `PrepareBodyOptWithEffortsAndDefault` 在强制 `stream:true` 之后、指纹脱敏之前调用
`normalizeZCodeFormat`，把 AI-SDK v5 / ZCode 形态折成标准 OpenAI：

- 内容块数组（`text` / `reasoning` / `thinking` / `tool-call` / `tool-result` / `image` / `file`）
  → 标准 messages（tool-result 拆成 `role:"tool"` + `tool_call_id`，图片转 `image_url` 部件）；
- 字典形态的 `tools` → `{"type":"function","function":{...}}` 数组；
- `maxOutputTokens` / `max_output_tokens` → `max_tokens`（不碰 `max_completion_tokens`）；
- **孤儿 tool 消息降级**：没有配对 `tool_call_id` 的 `tool` 消息改成 `user`，避免上游报 11148。

标准 OpenAI 请求原样通过（有测试锁定）。改这块时同步更新 `zcode_test.go` 与 `payload_test.go`。

## 请求日志环形缓冲（internal/server/reqlog.go）

进程内环形缓冲，容量 500，供面板「请求日志」页读。`logChatRow` 在 `chatLogEnabled` 早退**之前**
就写入缓冲，所以即使关掉 stdout 的表格日志，面板仍有记录；uid 只存前 8 位。

## Windows 部署

本 fork 的目标部署形态之一是 Windows 计划任务（另一套是 Docker / systemd）：

- 二进制为仓库根的 `server.exe`，由计划任务 `wbapi-gateway` 拉起；隧道是 `wbapi-tunnel` + `cloudflared.exe`。
- `gateway-task.cmd` / `tunnel-task.cmd` 是这两个任务的入口脚本，内容含**本机绝对路径**，已被
  `.gitignore` 排除，不进版本库（换机器要自己写一份）。
- `wbapi` 通过 `os.name == "nt"` 判断平台：服务命令改走 `schtasks` / `taskkill`，探活改成
  `/healthz` + `tasklist`，日志改读 `gateway.log` / `gateway.err.log`，`doctor` 跳过 uid 属主检查。
- Windows 上 `python3` 常被 Microsoft Store 的应用执行别名占用（执行无任何输出），
  跑 `wbapi` 用 `uv run --no-project python wbapi ...` 或指向真实解释器。

## 测试

- 后端：`go test ./...`。新增管理接口时在 `internal/admin/admin_test.go` 里加一条 httptest 用例，
  覆盖「无 token 401 / 动作生效 / 未知目标 404」三类断言；面板对话的代理行为在 `internal/admin/chat_test.go`。
- 密钥读写测试在 `internal/server/keystore_test.go`（新增/掩码/前缀吊销/坏文件不覆盖/热重载）。
- 请求日志环形缓冲的测试在 `internal/server/reqlog_test.go`（顺序、容量回绕、脱敏）。
- 请求体归一化测试在 `internal/upstream/zcode_test.go`。
- 前端：`pnpm build` 内含 `vue-tsc --noEmit` 类型检查；改完 UI 用 `pnpm dev` 或构建后进
  `/admin/` 实际点一遍（尤其动作按钮与空/错状态）。

## 提交前自检

```bash
gofmt -l $(git ls-files '*.go')   # 见下方说明：只看你改动的文件
go vet ./...
go test ./...
cd web && pnpm build              # 含类型检查
```

> 仓库里有相当一部分**历史遗留**的 gofmt 偏差文件（上游带来，40+ 个，多是结构体字段对齐）。
> 只格式化你改动的文件，别整仓重排制造无意义的 diff。

## 文档约定

| 文档 | 定位 | 长度约束 |
|---|---|---|
| `README.md` | 门面：是什么、怎么跑起来、文档索引 | **≤ 1500 字**（硬约束，宁短勿长） |
| `docs/*.md` | 详细内容：能力、部署、管理面、配置、合规 | 单篇 ≤ 1000 行 |
| `DEVELOPMENT.md` | 维护者：目录结构、构建链路、内部约定、测试 | — |

- README 只放「一眼看完」的信息，细节一律下沉到 `docs/`，两边靠文档索引互相链接；
  往 README 加内容前先想清楚它是不是该进 `docs/`。
- 改完 README 量一下：`wc -m README.md` 应 < 1500。
- `.gitignore` 用 `*.md` 排除 Markdown，白名单放行 `README.md`、`DEVELOPMENT.md` 与 `docs/*.md`；
  新增其它需要进版本库的文档，要加一条 `!路径.md`。
- 本机运维笔记 `NOTES.md` 属于个人环境信息（主机名、路径、域名），保持 gitignore 状态，不进版本库。
