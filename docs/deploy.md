# 部署

[← 返回 README](../README.md)

## 选择安装方式

推荐使用 GitHub Release 预构建包，不需要安装 Go。只有修改源码或参与开发时才需要本地构建。

| 方式 | 适用场景 | 额外要求 |
| --- | --- | --- |
| Release 预构建包 | 直接部署 | 无 |
| 源码构建 | 修改代码或自行编译 | Go 1.22+ |
| 前端开发 | 修改管理面板 | Node 20+、pnpm 9+ |
| KernelSU 模块 | Android 手机 | KernelSU、arm64-v8a |

本项目不提供 Docker 镜像。Android 手机不使用这里的预构建包，安装、目录与 CLI
见 [Android 模块](android-module.md)。

## Linux 一键安装

```bash
curl -fsSL https://raw.githubusercontent.com/YuleBest/workbuddy2api/master/install.sh | bash
```

安装器会完成以下操作：

1. 检测 Linux `amd64` 或 `arm64`。
2. 下载 `v1.0.1` Release 压缩包和 `SHA256SUMS`。
3. 校验 SHA-256。
4. 安装到 `~/.local/share/wb2api`。
5. 创建 `wb2api`、`wbapi` 和 `wb2api-login` 命令入口。
6. 首次安装时生成随机 `api_key`，启用管理面板并打印地址。

默认不会启动网关，也不会创建 systemd 服务、计划任务或 cloudflared 配置。

可以提前下载脚本检查：

```bash
curl -fsSLO https://raw.githubusercontent.com/YuleBest/workbuddy2api/master/install.sh
less install.sh
bash install.sh
```

可选环境变量：

| 变量 | 作用 |
| --- | --- |
| `WB2API_VERSION` | 安装指定版本，默认 `v1.0.1` |
| `WB2API_HOME` | 修改运行目录 |
| `WB2API_BIN_DIR` | 修改命令链接目录，默认 `~/.local/bin` |

## 下载 Release

每个 Release 提供以下运行包：

| 平台 | 架构 | 文件 |
| --- | --- | --- |
| Linux | `amd64` | `workbuddy2api-<tag>-linux-amd64.tar.gz` |
| Linux | `arm64` | `workbuddy2api-<tag>-linux-arm64.tar.gz` |
| macOS | `amd64` | `workbuddy2api-<tag>-darwin-amd64.tar.gz` |
| macOS | `arm64` | `workbuddy2api-<tag>-darwin-arm64.tar.gz` |
| Windows | `amd64` | `workbuddy2api-<tag>-windows-amd64.zip` |
| Windows | `arm64` | `workbuddy2api-<tag>-windows-arm64.zip` |

Release 同时包含 `SHA256SUMS`。压缩包内包含完整运行命令、`config.example.json`、README 和 LICENSE。

Android 模块的发布包由本 fork 的 Release 提供：`wb2api-<version>-arm64-v8a.zip`
及同名 `.sha256`，刷入 KernelSU 即可，步骤见 [Android 模块](android-module.md)。

Linux 或 macOS 校验单个下载文件：

```bash
grep 'linux-amd64' SHA256SUMS | sha256sum -c -
# macOS 使用：
grep 'darwin-arm64' SHA256SUMS | shasum -a 256 -c -
```

Windows PowerShell 校验：

```powershell
Get-FileHash .\workbuddy2api-<tag>-windows-amd64.zip -Algorithm SHA256
```

## Linux systemd 部署

下面以 `/home/your-user/wb2api` 为运行目录。不要把配置、凭证和日志放在 Git 源码目录。

```bash
mkdir -p /home/your-user/wb2api
tar -xzf workbuddy2api-<tag>-linux-amd64.tar.gz -C /home/your-user/wb2api
cd /home/your-user/wb2api
cp config.example.json config.json
```

编辑 `config.json`。至少设置：

```json
{
  "listen": "127.0.0.1:7863",
  "api_key": "replace-with-a-random-key"
}
```

添加账号：

```bash
./login
# 国际版账号：
./login --realm=global
```

登录完成后，凭证写入 `auths/`，运行中的网关会自动加载，不需要重启。

创建 `/etc/systemd/system/wbapi-gateway.service`：

```ini
[Unit]
Description=WorkBuddy2API Gateway
After=network-online.target
Wants=network-online.target

[Service]
User=your-user
WorkingDirectory=/home/your-user/wb2api
ExecStart=/home/your-user/wb2api/wb2api -config /home/your-user/wb2api/config.json
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
```

启用并检查：

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now wbapi-gateway
cd /home/your-user/wb2api
./wbapi doctor
curl -s http://127.0.0.1:7863/healthz
```

如果使用 Cloudflare Tunnel，请单独创建 `wbapi-tunnel.service`。`wbapi tunnel` 默认操作该服务名。

## macOS 运行

解压对应 `darwin` 包，例如到 `/Users/your-user/wb2api`：

```bash
mkdir -p /Users/your-user/wb2api
tar -xzf workbuddy2api-<tag>-darwin-arm64.tar.gz -C /Users/your-user/wb2api
cd /Users/your-user/wb2api
cp config.example.json config.json
./login
./wb2api -config "$PWD/config.json"
```

长期后台运行可以创建 `~/Library/LaunchAgents/com.yule.wb2api.plist`，在 `ProgramArguments` 中填写 `wb2api` 和配置文件的绝对路径，然后加载：

```bash
launchctl bootstrap "gui/$(id -u)" "$HOME/Library/LaunchAgents/com.yule.wb2api.plist"
```

## Windows 计划任务

解压 Windows 包到运行目录，例如 `C:\wb2api`，然后：

```powershell
Copy-Item config.example.json config.json
# 编辑 config.json，建议设置 listen=127.0.0.1:7863 和 api_key
.\login.exe
```

创建两个登录时启动的计划任务：

```powershell
schtasks /Create /TN wbapi-gateway /SC ONLOGON /TR "\"C:\wb2api\server.exe\" -config C:\wb2api\config.json" /F
schtasks /Create /TN wbapi-tunnel  /SC ONLOGON /TR "\"C:\cloudflared\cloudflared.exe\" tunnel run" /F
```

常用命令：

```powershell
.\wbapi.exe status
.\wbapi.exe start
.\wbapi.exe restart
.\wbapi.exe log 50
```

Windows 日志读取运行目录中的 `gateway.log` 和 `gateway.err.log`。

## 源码构建

环境要求：

- Go 1.22+
- 仅在修改 `web/` 时需要 Node 20+ 和 pnpm 9+

克隆源码后构建：

```bash
git clone https://github.com/YuleBest/workbuddy2api.git
cd workbuddy2api
go run ./cmd/build
```

产物默认写入 `dist/`，包括服务端、运维 CLI、登录、任务和兼容命令名。

修改了管理面板时，先构建前端：

```bash
cd web
pnpm install
pnpm build
cd ..
go run ./cmd/build
```

运行完整检查：

```bash
go test ./...
go vet ./...
```

## 源码与运行目录分离

开发目录只保存源码、构建产物和 Git 数据；生产配置、凭证、日志和正在运行的二进制应放在独立运行目录。

```text
源码目录：~/projects/wb2api
运行目录：~/.local/share/wb2api
```

运行目录通常包含：

```text
wb2api
wbapi
login
config.json
keys.json
auths/
data/
cloudflared/
```

升级时先在源码目录构建到临时 staging，再通过服务管理流程把二进制原子安装到运行目录并重启。
不要在源码目录直接运行生产网关，也不要在服务运行期间逐文件覆盖二进制。

## 发布流程

发布 GitHub Release 后，`.github/workflows/release.yml` 会自动构建 6 个平台包并上传 `SHA256SUMS`。
普通 push 和 Pull Request 只运行测试，不生成发布包。

## 验证

先用 CLI 检查服务和账号：

```bash
./wbapi status
./wbapi accounts
./wbapi doctor
```

再验证模型接口：

```bash
curl -s http://127.0.0.1:7863/v1/models \
  -H "Authorization: Bearer your-api-key"
```

发起流式请求：

```bash
curl -sN http://127.0.0.1:7863/v1/chat/completions \
  -H "Authorization: Bearer your-api-key" \
  -H "Content-Type: application/json" \
  -d '{"model":"deepseek-v4-flash","messages":[{"role":"user","content":"hi"}],"stream":true}'
```

发起非流式请求：

```bash
curl -s http://127.0.0.1:7863/v1/chat/completions \
  -H "Authorization: Bearer your-api-key" \
  -H "Content-Type: application/json" \
  -d '{"model":"deepseek-v4-flash","messages":[{"role":"user","content":"hi"}],"stream":false}'
```
