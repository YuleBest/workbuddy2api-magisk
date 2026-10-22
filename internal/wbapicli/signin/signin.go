// Package signin implements the one-shot batch check-in behavior shared by the
// Go checkin wrapper. It intentionally mirrors cmd/signin's output and status
// classification without invoking another executable.
package signin

import (
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"workbuddy2api/internal/auth"
	"workbuddy2api/internal/logfmt"
	"workbuddy2api/internal/upstream"
)

type Row struct {
	File     string
	UID      string
	Nick     string
	Status   string
	Detail   string
	Remain   int64
	HasQuota bool
}

type Client interface {
	RefreshToken(*auth.Auth) error
	DailyCheckin(*auth.Auth) error
	UserResource(*auth.Auth) (int64, error)
}

type UpstreamClient struct {
	Client *upstream.Client
}

func NewClient() *UpstreamClient {
	client := upstream.New()
	client.GlobalEnabled = true
	return &UpstreamClient{Client: client}
}

func (c *UpstreamClient) RefreshToken(a *auth.Auth) error {
	return c.Client.RefreshToken(a)
}

func (c *UpstreamClient) DailyCheckin(a *auth.Auth) error {
	return c.Client.DailyCheckin(a)
}

func (c *UpstreamClient) UserResource(a *auth.Auth) (int64, error) {
	return c.Client.UserResource(a)
}

var ErrNoAuthFiles = errors.New("no auth files")

type IO struct {
	ReadFile    func(string) ([]byte, error)
	LoadFiles   func(string) ([]string, error)
	SaveAtomic  func(*auth.Auth) error
	Parse       func([]byte) (*auth.Auth, error)
	Backfill    func(*auth.Auth) (bool, string)
	Logf        func(string, ...any)
	NeedsExpiry time.Duration
}

func defaultIO() IO {
	return IO{
		ReadFile:    os.ReadFile,
		LoadFiles:   auth.LoadAuthFiles,
		SaveAtomic:  func(a *auth.Auth) error { return a.SaveAtomic() },
		Parse:       auth.Parse,
		Backfill:    func(a *auth.Auth) (bool, string) { return a.BackfillRealm() },
		Logf:        log.Printf,
		NeedsExpiry: 2 * time.Hour,
	}
}

type Summary struct {
	Rows     []Row
	OK       int
	Already  int
	Fail     int
	LoadFail int
}

func Collect(dir string, client Client) (Summary, error) {
	return CollectWithIO(dir, client, defaultIO())
}

func CollectWithIO(dir string, client Client, deps IO) (Summary, error) {
	defaults := defaultIO()
	if deps.ReadFile == nil {
		deps.ReadFile = defaults.ReadFile
	}
	if deps.LoadFiles == nil {
		deps.LoadFiles = defaults.LoadFiles
	}
	if deps.SaveAtomic == nil {
		deps.SaveAtomic = defaults.SaveAtomic
	}
	if deps.Parse == nil {
		deps.Parse = defaults.Parse
	}
	if deps.Backfill == nil {
		deps.Backfill = defaults.Backfill
	}
	if deps.Logf == nil {
		deps.Logf = defaults.Logf
	}
	if deps.NeedsExpiry == 0 {
		deps.NeedsExpiry = defaults.NeedsExpiry
	}

	files, err := deps.LoadFiles(dir)
	if err != nil {
		return Summary{}, fmt.Errorf("%w in %s: %v", ErrNoAuthFiles, dir, err)
	}
	if len(files) == 0 {
		return Summary{}, fmt.Errorf("%w in %s", ErrNoAuthFiles, dir)
	}
	var summary Summary
	for _, file := range files {
		row := Row{File: filepath.Base(file)}
		raw, err := deps.ReadFile(file)
		if err != nil {
			row.Status, row.Detail = "LOAD_ERR", err.Error()
			summary.Rows = append(summary.Rows, row)
			summary.Fail++
			summary.LoadFail++
			continue
		}
		a, err := deps.Parse(raw)
		if err != nil {
			row.Status, row.Detail = "LOAD_ERR", err.Error()
			summary.Rows = append(summary.Rows, row)
			summary.Fail++
			summary.LoadFail++
			continue
		}
		a.FilePath = file
		row.UID, row.Nick = a.UID, a.Nickname

		if a.NeedsRefresh(deps.NeedsExpiry) {
			if err := client.RefreshToken(a); err != nil {
				row.Status = RefreshStatusOf(err)
				row.Detail = "refresh: " + Short(err.Error())
				summary.Rows = append(summary.Rows, row)
				summary.Fail++
				continue
			}
			_, _ = deps.Backfill(a)
			if err := deps.SaveAtomic(a); err != nil {
				deps.Logf("signin %s save: %v", a.UID, err)
			}
		}

		err = client.DailyCheckin(a)
		row.Status = CheckinStatusOf(err)
		if err != nil {
			row.Detail = Short(err.Error())
		}
		switch row.Status {
		case "OK":
			summary.OK++
		case "ALREADY":
			summary.Already++
		default:
			summary.Fail++
		}
		if remain, quotaErr := client.UserResource(a); quotaErr == nil {
			row.Remain, row.HasQuota = remain, true
		}
		summary.Rows = append(summary.Rows, row)
	}
	return summary, nil
}

func Render(w io.Writer, summary Summary) {
	fmt.Fprintln(w, "uid                                  | nick        | status       | remain | detail")
	fmt.Fprintln(w, "-------------------------------------+-------------+--------------+--------+------------------------------")
	for _, row := range summary.Rows {
		remain := "-"
		if row.HasQuota {
			remain = fmt.Sprintf("%d", row.Remain)
		}
		fmt.Fprintf(w, "%-36s | %-11s | %-12s | %-6s | %s\n",
			logfmt.Truncate(row.UID, 36), logfmt.Truncate(row.Nick, 11),
			row.Status, remain, row.Detail)
	}
	fmt.Fprintf(w, "\ntotal=%d ok=%d already=%d fail=%d\n",
		len(summary.Rows), summary.OK, summary.Already, summary.Fail)
}

func RenderFailures(w io.Writer, summary Summary) {
	fmt.Fprintln(w, "── 问题明细（仅 fail/错误，已签到不计）──")
	found := false
	for _, row := range summary.Rows {
		switch row.Status {
		case "FAIL", "LOAD_ERR", "AUTH_INVALID":
			remain := "-"
			if row.HasQuota {
				remain = fmt.Sprintf("%d", row.Remain)
			}
			fmt.Fprintf(w, "%-36s | %-11s | %-12s | %-6s | %s\n",
				logfmt.Truncate(row.UID, 36), logfmt.Truncate(row.Nick, 11),
				row.Status, remain, row.Detail)
			found = true
		}
	}
	if !found {
		fmt.Fprintln(w, "  （无 fail）")
	}
}

var idempotentCodes = []string{"10001", "14001"}

var idempotentMarkers = []string{
	"今天已签到", "今日已签到", "已签到", "already",
	"未开启", "未开放", "已过期", "inactive",
}

var bareMarkers = []string{"今天已签到", "今日已签到", "已签到", "未开启", "未开放", "已过期"}

func IsAlready(err error) bool {
	var upstreamErr *upstream.Error
	if errors.As(err, &upstreamErr) {
		return idempotentMatch(upstreamErr.Msg)
	}
	if err == nil {
		return false
	}
	message := err.Error()
	for _, marker := range bareMarkers {
		if strings.Contains(message, marker) {
			return true
		}
	}
	return false
}

func idempotentMatch(message string) bool {
	low := strings.ToLower(message)
	for _, code := range idempotentCodes {
		for i := 0; i+len(code) <= len(low); {
			j := strings.Index(low[i:], code)
			if j < 0 {
				break
			}
			start, end := i+j, i+j+len(code)
			before, after := byte(0), byte(0)
			if start > 0 {
				before = low[start-1]
			}
			if end < len(low) {
				after = low[end]
			}
			if !codeWordChar(before) && !codeWordChar(after) {
				return true
			}
			i = end
		}
	}
	for _, marker := range idempotentMarkers {
		if strings.Contains(low, strings.ToLower(marker)) {
			return true
		}
	}
	return false
}

func codeWordChar(c byte) bool {
	return c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c == '_'
}

func RefreshStatusOf(err error) string {
	var upstreamErr *upstream.Error
	if errors.As(err, &upstreamErr) && upstreamErr.Kind == upstream.ErrSessionDead {
		return "AUTH_INVALID"
	}
	return "FAIL"
}

func CheckinStatusOf(err error) string {
	if err == nil {
		return "OK"
	}
	if IsAlready(err) {
		return "ALREADY"
	}
	return "FAIL"
}

func Short(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) > 60 {
		return s[:60]
	}
	return s
}
