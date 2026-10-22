package loginflow

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

func (r *Runner) runCheckin(ctx context.Context, token string, account Account, domain string) {
	checkinCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(
		checkinCtx,
		http.MethodPost,
		strings.TrimRight(r.CNBaseURL, "/")+"/v2/billing/meter/daily-checkin",
		bytes.NewReader([]byte("{}")),
	)
	if err != nil {
		fmt.Fprintf(r.Out, "签到: %v\n", err)
		return
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User-Id", account.UID)
	if account.EnterpriseID != "" {
		req.Header.Set("X-Enterprise-Id", account.EnterpriseID)
		req.Header.Set("X-Tenant-Id", account.EnterpriseID)
	}
	if domain != "" {
		req.Header.Set("X-Domain", domain)
	}
	resp, err := r.HTTPClient.Do(req)
	if err != nil {
		fmt.Fprintf(r.Out, "签到: %v\n", err)
		return
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Fprintf(r.Out, "签到: %v\n", err)
		return
	}
	body, parseErr := decodeMap(raw)
	if parseErr != nil {
		if resp.StatusCode >= 400 {
			fmt.Fprintf(r.Out, "签到: http %d\n", resp.StatusCode)
		} else {
			fmt.Fprintf(r.Out, "签到: %v\n", parseErr)
		}
		return
	}
	code, _ := numericCode(body["code"])
	if resp.StatusCode >= 200 && resp.StatusCode < 300 && code == 0 {
		data := body["data"]
		if data == nil {
			data = map[string]any{}
		}
		fmt.Fprintf(r.Out, "签到: 成功 %s\n", shortJSON(data, 150))
		return
	}
	fmt.Fprintf(r.Out, "签到: %s\n", responseMessage(body, fmt.Sprintf("http %d", resp.StatusCode)))
}
