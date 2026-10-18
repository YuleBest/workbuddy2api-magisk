# 部署

[← 返回 README](../README.md)

## 环境要求

- **Docker + Docker Compose**（推荐部署方式，镜像内已含 `app` 低权限用户与全部工具脚本），
  或 **Windows 10/11 + 计划任务**（本 fork 的部署形态）
- 一个或多个已注册的 CodeBuddy 账号，用于 OAuth 登录
- 宿主机 Go ≥ 1.22（仅源码构建时需要）、Node ≥ 20 + pnpm ≥ 9（仅改前端时需要）
- `wbapi` CLI 需要 Python 3

## Docker Compose 一键部署

```bash
git clone https://github.com/YuleBest/workbuddy2api.git
cd workbuddy2api
cp config.example.json config.json
```

编辑 `config.json`，**至少设置 `api_key`**（`留空 = 不鉴权`，公网部署务必设置）。示例中的 `test_key` 等均为占位符，`config.example.json` 不含任何真实密钥。

```bash
# 登录添加账号（重复执行可加多号；注意：执行过下方说明中的 chown 后，
# host 侧 login.sh 会被可写性预检拦截——此时请在容器内登录，见下方说明）
./login.sh

# 启动服务
docker compose up -d --build

# 健康检查（无可用账号时 503）；service 字段用于确认打到的是本网关
curl -s http://localhost:7863/healthz
# {"healthy":2,"total":3,"service":"workbuddy2api"}
```

`login.sh` 内置授权 URL 获取 + 浏览器登录 + token 轮询 + 首次签到 + `auths/workbuddy-<uid>.json` 落盘 + 容器重启，全程无 PKCE（state 由服务端签发）。账号池在容器启动时用 `auths/` 目录自动对齐，新增凭证文件即自动发现。

> **非 root 宿主用户注意**：`./login.sh` 以**当前宿主用户**落盘凭证（权限 0600），而容器内网关以 `app(uid 10001)` 读 + 回写（refresh / realm 补标识走 tmp+rename，需要目录写权限）。二者 uid 不同（例如 Linux 非 root 账号通常是 uid 1000）时容器读不到凭证文件，`/status` 账号数为 0——与 `./data` 卷的属主问题同源。登录后、启动前把目录属主交给 10001（root 或部署用户执行）：
>
> ```bash
> chown -R 10001:10001 ./auths
> ```
>
> 之后新增账号**必须**进**容器内**登录（`app` 自身落盘，属主即 10001，无需反复 chown；chown 后 host 侧 `./login.sh` 无写权限，脚本会在启动浏览器授权前直接退出并提示，不会白走一遍 OAuth。容器内无 docker CLI，完成后回宿主机重启）：
>
> ```bash
> docker compose exec -it wb2api bash -c './login.sh' && docker compose restart wb2api
> ```

## Windows 原生部署（计划任务）

不需要 Docker：构建出 `server.exe`，交给 Windows 计划任务托管，`wbapi` 负责日常操作。

```powershell
Copy-Item config.example.json config.json
# 编辑 config.json；建议把 listen 设为 127.0.0.1:7863，且务必设置 api_key

go build -trimpath -ldflags="-s -w" -o server.exe ./cmd/server
go build -trimpath -ldflags="-s -w" -o login.exe ./cmd/login
go build -trimpath -ldflags="-s -w" -o signin_bin.exe ./cmd/signin
go build -trimpath -ldflags="-s -w" -o credit.exe ./cmd/credit
```

把两个启动入口注册成计划任务（本仓库的约定名字是 `wbapi-gateway` / `wbapi-tunnel`，`wbapi` 的服务命令按这两个名字操作）：

- `gateway-task.cmd` — 启动 `server.exe -config config.json`，PID 写 `gateway.pid`，日志写 `gateway.log` / `gateway.err.log`
- `tunnel-task.cmd` — 启动 `cloudflared.exe` 隧道，PID 写 `tunnel.pid`

```powershell
# 以「登录时触发」注册（示例；按需改成开机触发或加延迟）
schtasks /Create /TN wbapi-gateway /SC ONLOGON /TR "C:\path\to\wb2api-fork\gateway-task.cmd" /F
schtasks /Create /TN wbapi-tunnel  /SC ONLOGON /TR "C:\path\to\wb2api-fork\tunnel-task.cmd"  /F
```

这两个 `.cmd` 的内容是本机绝对路径，已被 `.gitignore` 排除，请按自己的目录改写。之后统一用 `wbapi` 操作：

```bash
wbapi status      # 网关 / 隧道 / 池子状态
wbapi start       # = schtasks /Run /TN wbapi-gateway + tunnel
wbapi restart     # 重启（隧道会闪断几秒）
wbapi log 50      # 网关日志尾部
```

> `wbapi` 是 Python 3 脚本，shebang 依赖 `python3`。Windows 上 `python3` 常被 Microsoft Store 的
> 应用执行别名占用（**执行没有任何输出也不报错**），脚本就没法靠 shebang 直接跑。放一个薄包装到
> PATH 上，用真实解释器调它即可：
>
> ```sh
> #!/bin/sh
> # ~/bin/wbapi —— 用真实解释器调用仓库里的 wbapi
> exec uv run --no-project --python 3.12 python "C:/path/to/wb2api-fork/wbapi" "$@"
> ```
>
> 没有 uv 的话，把 `uv run --no-project --python 3.12 python` 换成任何真实 Python 3 的绝对路径。
> Linux / macOS 上 `python3` 通常没被劫持，直接 `./wbapi <命令>` 即可。

仓库里上游遗留的 `start-workbuddy2api.cmd` / `status-workbuddy2api.cmd` / `stop-workbuddy2api.cmd`
找的是 `wb2api.exe`（`data\server.*.log`、`wb2api.pid`），与本 fork 的 `server.exe` + 计划任务不是一套，
只用其中一个，别混用。

添加账号在 Git Bash 中运行 `login.sh`（它还负责 CN 首次签到以及 Global 注册地区 / trial 流程），
或在浏览器里用面板的「账号池」页观察结果。

## 源码构建

管理面板是 Vue 3 单页应用，构建产物经 `go:embed` 打进二进制（仓库内已含一份构建产物，只改后端时可直接编译）。**改动 `web/` 下的前端代码后**，要先重新构建前端：

```bash
cd web && pnpm install && pnpm build   # 产物落到 internal/admin/dist，被 go:embed 拾取
cd .. && go build ./cmd/server
```

开发前端时用 `pnpm dev` 起 Vite 开发服务器（已配置把 `/api/admin` 代理到 `127.0.0.1:7863`），改完再 `pnpm build` 落到二进制里。

```bash
go build ./...
go vet ./...
go test ./...      # 完整测试套件
go run ./cmd/server -config config.json
```

构建二进制：

```bash
CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o server ./cmd/server
CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o signin_bin ./cmd/signin
CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o login ./cmd/login
CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o credit ./cmd/credit
```

## 验证

```bash
# 模型列表
curl -s http://localhost:7863/v1/models -H "Authorization: Bearer your-api-key"

# 账号状态（汇总 + 每账号详情，含 disabled / manual_disabled 双位）
curl -s http://localhost:7863/status -H "Authorization: Bearer your-api-key"

# 临时停用一个账号（需 config 里 admin.enabled = true）
curl -s -X POST http://localhost:7863/admin/accounts/<uid>/disable \
  -H "Authorization: Bearer your-api-key" -H "Content-Type: application/json" \
  -d '{"reason":"观察几天"}'
# 或用 CLI（自动从 config.json 读网关地址与 key）
./acct.sh list && ./acct.sh disable <uid> 观察几天 && ./acct.sh enable <uid>

# 流式聊天
curl -sN http://localhost:7863/v1/chat/completions \
  -H "Authorization: Bearer your-api-key" \
  -H "Content-Type: application/json" \
  -d '{"model":"deepseek-v4-flash","messages":[{"role":"user","content":"hi"}],"stream":true}'

# 非流式聊天（本地聚合）
curl -s http://localhost:7863/v1/chat/completions \
  -H "Authorization: Bearer your-api-key" \
  -H "Content-Type: application/json" \
  -d '{"model":"deepseek-v4-flash","messages":[{"role":"user","content":"hi"}],"stream":false}'
```
