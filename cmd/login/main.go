// login.go — WorkBuddy OAuth 登录（设备授权流程，CN realm；--realm=global 供国际版）。
//
// 无子命令时运行完整登录流程；下列低层子命令保留给运维和自动化复用：
//
//	login [--realm=cn|global] url   → POST /v2/plugin/auth/state?platform=CLI 拿 state+authUrl，
//	                                  state 落 /tmp/wb2api-login-state.json，stdout 打印授权 URL
//	login [--realm=cn|global] poll  → 读 state，GET /v2/plugin/auth/token?state= 一次，
//	                                  成功再 GET /v2/plugin/login/account?state= 拿 uid/nickname，
//	                                  stdout 打印完整 token+account JSON（含 realm 键）
//
// --realm 默认 cn。按 realm 切换上游端点与 Origin/Referer：
//
//	cn     → https://copilot.tencent.com（Origin: https://www.codebuddy.cn）
//	global → https://www.workbuddy.ai（Origin: https://www.workbuddy.ai）
//
// state 落盘带 realm，poll 读回校验与命令行 --realm 一致（防混域）。
// 无 PKCE（workbuddy 设备流由服务端签发 state）。
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"os"
	"path/filepath"
	"strings"
	"time"

	auth2 "workbuddy2api/internal/auth"
	"workbuddy2api/internal/loginflow"
)

// 登录 state 落盘路径（var 便于测试替换临时文件）。
// 跨平台：os.TempDir() 在 Linux 解析为 /tmp，Windows 解析为
// 系统临时目录，避免硬编码 /tmp 在 Windows 上 "The system cannot find the path"。
var stateFile = filepath.Join(os.TempDir(), "wb2api-login-state.json")

// exitFunc 供测试替换（默认 os.Exit；测试持临时替换为 panic 以进程内捕获 fatal）。
var exitFunc = os.Exit

// realmConfig 按 realm 返回上游 base 与 Origin/Referer origin：global →
// (www.workbuddy.ai, www.workbuddy.ai)；cn/非法/缺省 → (copilot.tencent.com, codebuddy.cn)。
func realmConfig(realm string) (base, origin string) {
	return loginflow.RealmEndpoints(realm)
}

// commonHeaders 按 origin 设置通用请求头（Origin/Referer 随 realm 变化）。
// 返回 func(*http.Request)，由调用方按 realm 选定的 origin 构造一次后复用。
func commonHeaders(origin string) func(*http.Request) {
	return loginflow.CommonHeaders(origin)
}

// doJSON 与 upstream.doJSON 语义一致：{code,msg,data} 信封，code!=0 → error
func doJSON(client *http.Client, method, fullURL string, headers func(*http.Request), body io.Reader) (json.RawMessage, int, error) {
	return loginflow.DoJSON(client, method, fullURL, headers, body)
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "login: "+format+"\n", args...)
	exitFunc(1)
}

type loginState struct {
	State string `json:"state"`
	Realm string `json:"realm,omitempty"` // url 落盘时写回的 realm，poll 读回校验防混域
}

// realm 取值枚举（与 internal/auth 的 Realm() 归一化输出一致）。
const (
	realmCN     = "cn"
	realmGlobal = "global"
)

// parseRealmArgs 解析开头的 --realm=cn|global（或分离式 --realm <v>）flag，缺省 cn。
// 大小写不敏感归一化；非法值/缺值报错。桌椅剩余参数（子命令）顺序不变。
func parseRealmArgs(args []string) (realm string, rest []string, err error) {
	realm, _, rest, err = parseRealmArgsDetailed(args)
	return realm, rest, err
}

func parseRealmArgsDetailed(args []string) (realm string, explicit bool, rest []string, err error) {
	realm = realmCN
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--realm":
			if i+1 >= len(args) {
				return "", false, nil, fmt.Errorf("--realm requires a value")
			}
			v := strings.ToLower(strings.TrimSpace(args[i+1]))
			if v != realmCN && v != realmGlobal {
				return "", false, nil, fmt.Errorf("invalid --realm %q (want cn|global)", args[i+1])
			}
			realm = v
			explicit = true
			i++
		case strings.HasPrefix(a, "--realm="):
			v := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(a, "--realm=")))
			if v != realmCN && v != realmGlobal {
				return "", false, nil, fmt.Errorf("invalid --realm %q (want cn|global)", v)
			}
			realm = v
			explicit = true
		default:
			rest = append(rest, a)
		}
	}
	return realm, explicit, rest, nil
}

// resolveRealmInput 把交互式选域的一行输入归一化为 realm（纯函数，可测）。规则：
//
//	"1"/"cn"（大小写不敏感）/""（回车默认）→ cn
//	"2"/"global" → global
//	其他 → ("", false)（调用方回默认 cn）
func resolveRealmInput(input string) (string, bool) {
	return loginflow.ResolveRealmInput(input)
}

// promptRealm 交互式选域：向 out 打印选项提示（out 接 stderr，stdout 留给 realm 本身），
// 从 in 读一行，返回归一化 realm。非法输入警告后回落 cn；EOF（非交互/管道）回落 cn。
func promptRealm(in io.Reader, out io.Writer) string {
	return loginflow.PromptRealm(in, out)
}

// validateRealmMatch 校验 state 文件 realm 与命令行 --realm 一致（防混域）：
// state 无 realm（旧文件）放行；非空且不一致 → error。
func validateRealmMatch(stateRealm, cliRealm string) error {
	return loginflow.ValidateRealmMatch(stateRealm, cliRealm)
}

// runURL 执行 url 子命令：向 upstreamBase 的 state 端点 POST 取授权 URL，
// state 落盘（带 realm），stdout 打印 authURL。out 接 stdout；stateFile 为落盘路径
// （可注入临时文件便于测试）。空 realm 视为缺省（调用方已归一）。
func runURL(base, origin, realm, statePath string, client *http.Client, out io.Writer) {
	oauth := loginflow.NewOAuthClient(client, realm, base, origin, statePath)
	authURL, err := oauth.AuthURL(context.Background())
	if err != nil {
		fatal("%v", err)
	}
	fmt.Fprintln(out, authURL)
}

// runPoll 执行 poll 子命令：读 state 文件（realm 校验），向 upstreamBase 的 token 端点
// GET 一次，成功再 GET login/account（带 Bearer），stdout 打印完整 token+account JSON。
// statePath 可注入临时文件便于测试。
func runPoll(base, origin, realm, statePath string, client *http.Client, out io.Writer) {
	oauth := loginflow.NewOAuthClient(client, realm, base, origin, statePath)
	result, err := oauth.Poll(context.Background())
	if err != nil {
		fatal("%v", err)
	}
	var tok struct {
		AccessToken  string `json:"accessToken"`
		RefreshToken string `json:"refreshToken"`
		ExpiresIn    int64  `json:"expiresIn"`
		Domain       string `json:"domain"`
	}
	tok.AccessToken = result.Token.AccessToken
	tok.RefreshToken = result.Token.RefreshToken
	tok.ExpiresIn = result.Token.ExpiresIn
	tok.Domain = result.Token.Domain
	var acct struct {
		UID          string `json:"uid"`
		EnterpriseID string `json:"enterpriseId"`
		Nickname     string `json:"nickname"`
	}
	acct.UID = result.Account.UID
	acct.EnterpriseID = result.Account.EnterpriseID
	acct.Nickname = result.Account.Nickname
	oraw, _ := json.Marshal(buildLoginOutput(tok, realm, acct))
	fmt.Fprintln(out, string(oraw))
}

// buildLoginOutput 组装 poll 输出的完整 JSON。
// realm 永不空：显式 --realm 优先（ResolveRealm 处理），否则按上游返回的 domain 推断——
// 保证登录落盘的 auth 文件恒带 realm 键。
func buildLoginOutput(tok struct {
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken"`
	ExpiresIn    int64  `json:"expiresIn"`
	Domain       string `json:"domain"`
}, realm string, acct struct {
	UID          string `json:"uid"`
	EnterpriseID string `json:"enterpriseId"`
	Nickname     string `json:"nickname"`
}) map[string]any {
	return map[string]any{
		"access_token":  tok.AccessToken,
		"refresh_token": tok.RefreshToken,
		"expires_in":    tok.ExpiresIn,
		"domain":        tok.Domain,
		"realm":         auth2.ResolveRealm(realm, tok.Domain),
		"uid":           acct.UID,
		"enterprise_id": acct.EnterpriseID,
		"nickname":      acct.Nickname,
	}
}

func main() {
	realm, explicit, rest, err := parseRealmArgsDetailed(os.Args[1:])
	if err != nil {
		fatal("%v (usage: login [--realm=cn|global] <url|poll|realm>)", err)
	}
	if len(rest) < 1 {
		runner := loginflow.NewRunner()
		runner.Realm = realm
		runner.RealmExplicit = explicit
		runner.OAuthStatePath = stateFile
		home := runtimeHome()
		if v := os.Getenv("WB2A_AUTHS"); v != "" {
			runner.AuthDir = v
		} else {
			runner.AuthDir = filepath.Join(home, "auths")
		}
		os.Exit(runner.Run(context.Background()))
	}
	// 每个流程独立 cookie jar（多账号登录互不串会话）
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Timeout: 30 * time.Second, Jar: jar}

	base, origin := realmConfig(realm)

	switch rest[0] {
	case "url":
		runURL(base, origin, realm, stateFile, client, os.Stdout)

	case "poll":
		runPoll(base, origin, realm, stateFile, client, os.Stdout)

	case "realm":
		// 交互式选域；提示打到 stderr，stdout 只输出归一化 realm。
		realm := promptRealm(os.Stdin, os.Stderr)
		fmt.Println(realm)

	default:
		fatal("unknown subcommand %q (want url|poll|realm)", rest[0])
	}
}

func runtimeHome() string {
	if value := strings.TrimSpace(os.Getenv("WB2A_HOME")); value != "" {
		return value
	}
	executable, err := os.Executable()
	if err != nil {
		return "."
	}
	if real, err := filepath.EvalSymlinks(executable); err == nil {
		executable = real
	}
	return filepath.Dir(executable)
}
