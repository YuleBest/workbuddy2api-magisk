# 开发说明

本文面向修改代码、构建发布包和维护仓库的开发者。安装与使用请看 [`README.md`](README.md) 和 [`docs/android-module.md`](docs/android-module.md)。

## 项目边界

- 服务端、CLI、登录、任务和发布工具全部使用 Go。
- `web/` 使用 Vue 3、TypeScript 和 Vite，只允许在管理面板构建链路中使用。
- 运行时不能依赖 Python、Shell、Batch 或 Node。
- 仓库根 `install.sh` 是唯一的 Shell 引导器，只负责下载、校验和解压 Release 包。
- 本 fork 额外提供 KernelSU 模块：`module/` 生命周期脚本与 `scripts/` 构建脚本是仅有的 Shell 源码，白名单在 `internal/reposource/source_policy_test.go`。
- 项目不提供 Docker 支持，除非后续明确重新引入。
- 修改核心行为时必须保留 OpenAI 兼容接口和现有配置键语义。

## 目录结构

```text
cmd/server          网关入口
cmd/wbapi           桌面 / 服务器运维 CLI（internal/wbapicli 的入口）
cmd/wbapi-android   Android 模块管理 CLI（模块内产物名为 bin/wbapi）
cmd/login           OAuth 设备授权登录
cmd/build           全量构建和兼容命令生成
cmd/signin          签到
cmd/credit          积分查询
cmd/acct            账号停用、恢复和复活
cmd/activity        活跃上报
cmd/trial           国际版注册和 trial
cmd/stats           终端统计工具
cmd/school          开学季任务
cmd/taskrunner      通用任务执行器
cmd/taskcron        任务 cron 入口
cmd/region          国际版地区设置
internal/admin      管理接口、动作和嵌入式 SPA
internal/auth       凭证解析和原子写回
internal/config     共享配置结构
internal/loginflow  登录流程
internal/pool       账号池、选号、冷却和状态
internal/redisstore Redis 状态镜像
internal/scheduler  定时任务调度
internal/server     OpenAI 兼容数据面和管理 API
internal/session    会话粘性
internal/tasks      任务执行和活动协议（原 Python 任务已迁入）
internal/timezone   Android 时区初始化
internal/upstream   上游请求、SSE 和改写管线
internal/wbapicli   桌面 CLI 实现和帮助系统
module              KernelSU 模块脚本、配置模板与元数据
scripts             Android 构建与安装迁移测试脚本
web                 Vue 3 管理面板源码
```

## 环境要求

- Go 1.22+，`go.mod` 当前声明 `1.22.5`
- Node 20+ 和 pnpm 9+，仅修改前端时需要
- 构建 Android 模块：NDK r27c、`zip`、`shellcheck`、`shfmt`
- 一个已授权账号，仅在真实联调时需要

## 常用命令

### 后端

```bash
go test ./...
go vet ./...
go run ./cmd/server -config config.json
go run ./cmd/build
```

`go run ./cmd/build` 将服务端、CLI、登录、任务和兼容命令构建到 `dist/`。

### Android 模块

```bash
make build   # 交叉编译并打包到 build/stage 与 build/wb2api-<version>-arm64-v8a.zip
make test    # Go 测试、脚本 lint、安装迁移测试和模块校验
```

### 前端

```bash
cd web
pnpm install
pnpm dev
pnpm build
pnpm build:only
```

- `pnpm dev` 启动 Vite 开发服务器，并把管理 API 代理到 `127.0.0.1:7863`。
- `pnpm build` 执行 TypeScript 检查并写入 `internal/admin/dist`。
- `pnpm build:only` 跳过类型检查，只适合快速本地预览。

## Android 模块

构建入口是 `scripts/build_android.sh`，使用 NDK clang、`CGO_ENABLED=1`、
`-tags netcgo` 和目标 API 23：

```text
GOOS=android GOARCH=arm64 CGO_ENABLED=1 -tags netcgo
```

必须使用 cgo resolver：Android 没有常规 `/etc/resolv.conf`，纯 Go DNS 不可靠。
Android 下 Go 默认使用 UTC，因此 `cmd/server` 与 `cmd/wbapi-android` 都调用
`internal/timezone.Init()` 固定为 `Asia/Shanghai`。

运行时文件全部位于 `/data/adb/modules/wb2api`：

| 路径 | 内容 |
| --- | --- |
| `config.json` | 安装时由 `module/config.android.json` 生成，内含随机 `api_key` |
| `keys.json` | 多 key 存储 |
| `auths/` | 账号凭证 |
| `data/` | `state.json`、`requests.json`、`metrics.json` |
| `logs/gateway.log` | 网关日志，超过 4MB 轮转 |
| `run/wbapi.pid` | 网关 PID |

`cmd/wbapi-android/paths.go` 从自身路径推导模块根目录，并拒绝配置中指向模块
目录外的 `auth_dir`、`keys_file` 和 `state_file`。服务启动使用 `Setsid`，
`module/service.sh` 只调用 `wbapi start --quiet` 后立即返回。

`module/service.sh` 设置 `WB2A_SCHEDULER=0` 关闭自动调度；手动任务仍可从
面板或 `wbapi task` 触发。任务实现全部在 `internal/tasks`，不再有 Python
依赖，`school` 与 `cat` 与其他任务一样按需执行。

Android CLI 与桌面版 `cmd/wbapi` 的命令面基本一致，差异是：不管理
cloudflared 隧道，`start|stop|restart` 使用模块 PID 文件而不是 systemd。
新增命令时优先只改 `cmd/wbapi-android`，不要把 Android 专用逻辑塞进
`internal/wbapicli`。

## 前端构建链路

```text
web/src --pnpm build--> internal/admin/dist --go:embed--> Go binary
```

约定：

- `internal/admin/dist` 是嵌入二进制的构建产物，需要提交。
- 修改 `web/` 后必须重新执行 `pnpm build`，再把前后端改动一起提交。
- 管理面板路由固定为 `/admin/`，Go 前缀和 `vite.config.ts` 的 `base` 必须一致。
- 未登录只允许访问 `/admin/login`，鉴权状态来自 `GET /api/admin/session`。
- 静态资源哈希文件使用长缓存，其他文件使用 `no-cache`。

## 管理入口

网关有两组管理入口，但底层状态必须复用同一批方法。

| 入口 | 路径 | 客户端 |
| --- | --- | --- |
| 兼容管理端点 | `/admin/accounts/{uid}/{disable,enable,revive}` | `acct`、`wbapi acct` |
| 管理 API | `/api/admin/*` | Web 面板、`wbapi` |

账号状态约定：

- `manual_disabled` 表示人工停用。
- `disabled` 表示系统自动禁用。
- `enable` 只清除手动停用。
- `revive` 只清除系统禁用。
- 两个状态都为空时，账号才回到选号池。

新增管理动作时：

1. 优先复用 `pool`、`scheduler` 或 `server.KeyStore` 的现有方法。
2. 同步检查 `/api/admin/*` 和兼容 `/admin/*` 是否需要暴露。
3. 添加无 token、成功、未知目标和失败路径测试。

## 管理 API

- 所有接口位于 `/api/admin/*`。
- 鉴权 token 优先级为 `admin.token`、`keys.json` 首个 key、`api_key`。
- 响应统一使用 JSON。
- 错误格式为 `{"error":{"code":"...","message":"..."}}`。
- `config` 接口必须脱敏。
- 管理 pprof 或其他调试端点如果存在，必须沿用同一鉴权边界。

面板模型对话入口为 `POST /api/admin/v1/chat/completions`。它必须继续走数据面 handler，不能旁路选号、粘性、冷却、日志和统计。

## 调用密钥

文件格式：

```json
{
  "keys": [
    {
      "key": "sk-...",
      "note": "备注",
      "added": "2026-10-20"
    }
  ]
}
```

实现位于 `internal/server/keystore.go`：

- 路径取 `keys_file`，默认 `keys.json`。
- 文件变化按 mtime 热加载。
- 文件缺失或为空时回落 `api_key`。
- 解析失败时保留上一份可用 key。
- 写入使用临时文件加原子替换，权限为 `0600`。

## 请求改写

`internal/upstream` 统一处理下游请求到上游协议的转换：

- 强制上游使用流式请求，非流式响应由网关本地聚合。
- 兼容 AI-SDK、ZCode 和其他 OpenAI 兼容客户端的字段差异。
- 归一内容块、工具定义、`tool_choice`、图片和推理字段。
- 处理孤立 tool 消息，避免上游 11148。
- 注入 DeepSeek thinking 和 reasoning 字段。
- 可选清洗请求体指纹。

修改这段逻辑时，必须补充表驱动测试，并覆盖标准 OpenAI 请求的零回归行为。

## 持久化

默认数据目录为 `./data`（Android 模块为 `/data/adb/modules/wb2api/data`）：

| 文件 | 内容 |
| --- | --- |
| `state.json` | 账号池状态、冷却、积分和成本账本 |
| `requests.json` | 最近 500 条请求日志 |
| `metrics.json` | Token、延迟和模型统计 |

这些文件均使用临时文件加原子替换。测试持久化逻辑时同时覆盖写入失败、恢复和损坏文件回退。

## CLI 帮助

桌面 CLI 的帮助唯一来源是 `internal/wbapicli/help.go`。新增或修改命令时：

1. 更新命令帮助和子命令帮助。
2. 支持 `wbapi help <命令>` 和 `wbapi <命令> --help`。
3. 参数错误返回 `2`，并提示具体帮助命令。
4. 更新 `internal/wbapicli/help_test.go`。
5. 同步检查 [`docs/admin.md`](docs/admin.md) 的命令表。

Android CLI 的帮助文本在 `cmd/wbapi-android/main.go`，修改命令后同步更新
[`docs/android-module.md`](docs/android-module.md)。

## 上游同步

本 fork 与上游历史互相独立（fork 首个提交是整树导入），不能直接
`git rebase` 或 `git merge`。同步方式是以 `upstream/master` 为基线重建工作树，
再重放 fork 增量：

```bash
git fetch upstream                    # upstream = YuleBest/workbuddy2api
git worktree add /tmp/wb-sync HEAD
cd /tmp/wb-sync
git checkout -B sync-work HEAD
git read-tree -u --reset upstream/master
# 恢复 fork 专有文件（module/、scripts/、cmd/wbapi-android 等），
# 再合并共享文件的 Android 适配（见下）
```

需要重放的 fork 适配点：

- `cmd/server/main.go`：`internal/timezone.Init()`、`WB2A_SCHEDULER=0` 网关开关、管理后台地址日志。
- `internal/server/handler.go`：`WB2A_DUMP_REQ` 落盘到 `os.TempDir()`。
- `internal/reposource/source_policy_test.go`：模块与构建脚本的 Shell 白名单。
- `cmd/wbapi-android/`、`module/`、`scripts/`、`docs/android-module.md`、`.github/workflows/build-android.yml`、`Makefile`、`CHANGELOG.md`。
- `README.md`、`DEVELOPMENT.md`、`docs/` 的 Android 说明。

同步后至少跑 `make test`、`cd web && pnpm build`，并在真机验证安装、重启、
Action 按钮、CLI、管理后台、OpenAI 请求和升级迁移。

## 发布

打 `v*` 标签会触发 `.github/workflows/build-android.yml`：交叉编译模块、
校验 ZIP 并发布 Release；`CHANGELOG.md` 顶部小节作为发布说明，标签必须与
`module/module.prop` 的 `version` 完全一致。上游的 `ci.yml` 负责 Go 测试，
`release.yml` 负责桌面平台发布包。

## 测试

最低要求：

```bash
go test ./...
go vet ./...
```

按风险补充测试：

- 账号池和并发行为：`go test -race ./internal/pool/ ./internal/server/ ./internal/session/`
- 管理接口：`internal/admin/*_test.go`
- 密钥热加载和权限：`internal/server/keystore_test.go`
- 请求日志和统计：`internal/server/reqlog_test.go`、`metrics_test.go`
- 请求体归一化：`internal/upstream/*_test.go`
- 任务系统：`internal/tasks/*_test.go`
- CLI 帮助：`internal/wbapicli/help_test.go`
- Android 安装迁移：`scripts/test_customize.sh`

## 提交前检查

```bash
gofmt -l $(git ls-files '*.go')   # 只格式化本次修改的文件
go vet ./...
go test ./...
cd web && pnpm build
make build
```

仓库仍有部分历史 gofmt 偏差文件。只格式化本次修改的 Go 文件，不要制造全仓库格式 diff。

## 文档约定

| 文档 | 定位 |
| --- | --- |
| `README.md` | 项目简介、快速开始和文档导航，限制在 1500 字符内 |
| `docs/android-module.md` | Android 安装、目录、CLI 与构建 |
| `docs/deploy.md` | 桌面和服务器的安装、服务与源码构建 |
| `docs/config.md` | 配置键和生效规则 |
| `docs/admin.md` | 管理面板、密钥和 CLI |
| `docs/features.md` | 核心能力和架构 |
| `docs/compliance.md` | 安全边界、免责声明和许可 |
| `DEVELOPMENT.md` | 开发、测试、内部契约和发布流程 |

文档应优先使用短段落、小标题、表格和可复制命令。不要加入本机私有路径、域名、token 或运维笔记。
