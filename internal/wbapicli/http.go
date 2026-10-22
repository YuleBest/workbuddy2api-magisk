package wbapicli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

func (a *App) request(path, token, method string, body any, timeout time.Duration) (int, map[string]any) {
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return 0, errorEnvelope(err.Error())
		}
		reader = bytes.NewReader(raw)
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, method, a.baseURL()+path, reader)
	if err != nil {
		return 0, errorEnvelope(err.Error())
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	client := a.deps.HTTPClient
	resp, err := client.Do(req)
	if err != nil {
		return 0, errorEnvelope(err.Error())
	}
	defer resp.Body.Close()
	raw, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return 0, errorEnvelope(readErr.Error())
	}
	if resp.StatusCode >= 400 {
		if data, parseErr := decodeObject(raw); parseErr == nil {
			return resp.StatusCode, data
		}
		msg := strings.ToValidUTF8(string(raw), "\uFFFD")
		if runes := []rune(msg); len(runes) > 200 {
			msg = string(runes[:200])
		}
		return resp.StatusCode, errorEnvelope(msg)
	}
	if len(raw) == 0 {
		return resp.StatusCode, map[string]any{}
	}
	data, err := decodeObject(raw)
	if err != nil {
		return 0, errorEnvelope(err.Error())
	}
	return resp.StatusCode, data
}

func (a *App) rawRequest(method, url, token string, body any, timeout time.Duration) ([]byte, int, error) {
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return nil, 0, err
		}
		reader = bytes.NewReader(raw)
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, method, url, reader)
	if err != nil {
		return nil, 0, err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := a.deps.HTTPClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	return raw, resp.StatusCode, err
}

func errorEnvelope(msg string) map[string]any {
	return map[string]any{"error": map[string]any{"message": msg}}
}

func errMessage(data map[string]any, fallback string) string {
	if data != nil {
		if obj, ok := data["error"].(map[string]any); ok {
			if msg, ok := obj["message"]; ok && !pyFalsy(msg) {
				return stringValue(msg)
			}
		}
	}
	return fallback
}

func (a *App) adminRequest(path, method string, body any) (int, map[string]any, int) {
	if !strings.HasPrefix(path, "/api/admin") {
		path = "/api/admin" + path
	}
	st, data := a.request(path, a.adminToken(), method, body, 90*time.Second)
	switch st {
	case http.StatusUnauthorized:
		return st, data, a.fail("管理 token 无效：检查 config.admin.token 或 keys.json 首个 key")
	case http.StatusNotFound:
		return st, data, a.fail("管理面未开启：config.json 里设 \"admin\": {\"enabled\": true} 后重启网关")
	case 0:
		return st, data, a.fail(fmt.Sprintf("连不上网关（%s）：%s", a.baseURL(), errMessage(data, "请求失败")))
	}
	return st, data, 0
}

func (a *App) fail(msg string) int {
	fmt.Fprintf(a.stdout, "[!] %s\n", msg)
	return 1
}
