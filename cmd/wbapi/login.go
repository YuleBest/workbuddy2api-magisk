package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type loginResult struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
	Domain       string `json:"domain"`
	Realm        string `json:"realm"`
	UID          string `json:"uid"`
	EnterpriseID string `json:"enterprise_id"`
	Nickname     string `json:"nickname"`
}

func (e *moduleEnv) cmdLogin(args []string) error {
	realm := "cn"
	for _, arg := range args {
		switch {
		case strings.HasPrefix(arg, "--realm="):
			realm = strings.TrimPrefix(arg, "--realm=")
		case arg == "--realm":
			return fmt.Errorf("请使用 --realm=cn 或 --realm=global")
		default:
			return fmt.Errorf("未知参数: %s", arg)
		}
	}
	if realm != "cn" && realm != "global" {
		return fmt.Errorf("realm 必须为 cn 或 global")
	}
	if _, err := os.Stat(e.LoginBin); err != nil {
		return fmt.Errorf("登录助手不存在: %s", e.LoginBin)
	}
	if err := e.prepareRuntimeDirs(); err != nil {
		return err
	}

	urlOutput, err := e.runLoginHelper("url", realm)
	if err != nil {
		return err
	}
	authURL := strings.TrimSpace(string(urlOutput))
	if authURL == "" {
		return fmt.Errorf("登录助手没有返回授权地址")
	}
	fmt.Printf("请在浏览器完成 %s 登录:\n\n%s\n\n", realm, authURL)
	if err := exec.Command("am", "start", "-a", "android.intent.action.VIEW", "-d", authURL).Run(); err != nil {
		fmt.Println("未能自动打开浏览器，请手动打开上面的地址")
	}
	fmt.Print("登录完成后按回车继续: ")
	_, _ = bufio.NewReader(os.Stdin).ReadString('\n')

	fmt.Println("正在等待授权结果...")
	deadline := time.Now().Add(90 * time.Second)
	var result loginResult
	for time.Now().Before(deadline) {
		output, err := e.runLoginHelper("poll", realm)
		if err == nil {
			if err := json.Unmarshal(output, &result); err != nil {
				return fmt.Errorf("解析登录结果: %w", err)
			}
			break
		}
		if !strings.Contains(string(output), "登录未完成") {
			return fmt.Errorf("获取授权失败: %s", strings.TrimSpace(string(output)))
		}
		fmt.Print(".")
		time.Sleep(2 * time.Second)
	}
	fmt.Println()
	if result.AccessToken == "" || result.UID == "" {
		return fmt.Errorf("登录超时或未取得有效凭证")
	}
	if strings.ContainsAny(result.UID, `/\`) || result.UID == "." || result.UID == ".." {
		return fmt.Errorf("登录结果包含非法 uid: %q", result.UID)
	}

	auth := map[string]any{
		"account": map[string]any{
			"uid":          result.UID,
			"enterpriseId": result.EnterpriseID,
			"nickname":     result.Nickname,
		},
		"auth": map[string]any{
			"accessToken":  result.AccessToken,
			"refreshToken": result.RefreshToken,
			"expiresAt":    time.Now().Unix() + result.ExpiresIn,
			"domain":       result.Domain,
			"realm":        result.Realm,
		},
	}
	raw, err := json.MarshalIndent(auth, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	authFile := filepath.Join(e.AuthDir, "workbuddy-"+result.UID+".json")
	if err := writeFileAtomic(authFile, raw, 0o600); err != nil {
		return err
	}
	fmt.Printf("已保存账号 %s（%s）\n", result.Nickname, shortUID(result.UID))
	fmt.Println("账号目录由服务自动热加载")
	return nil
}

func (e *moduleEnv) runLoginHelper(command, realm string) ([]byte, error) {
	cmd := exec.Command(e.LoginBin, "--realm="+realm, command)
	cmd.Dir = e.ModuleDir
	cmd.Env = environmentWith(map[string]string{
		"HOME":   e.ModuleDir,
		"TMPDIR": e.TmpDir,
		"TZ":     "Asia/Shanghai",
	})
	output, err := cmd.CombinedOutput()
	if err != nil {
		return output, fmt.Errorf("%s", strings.TrimSpace(string(output)))
	}
	return output, nil
}
