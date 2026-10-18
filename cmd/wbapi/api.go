package main

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

func (e *moduleEnv) request(path, token, method string, body any, timeout time.Duration) (int, []byte, error) {
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return 0, nil, err
		}
		reader = bytes.NewReader(raw)
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, method, e.baseURL()+path, reader)
	if err != nil {
		return 0, nil, err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, nil, err
	}
	return resp.StatusCode, raw, nil
}

func decodeObject(raw []byte) (map[string]any, error) {
	var value map[string]any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, err
	}
	return value, nil
}

func apiError(raw []byte) string {
	value, err := decodeObject(raw)
	if err == nil {
		if errObj, ok := value["error"].(map[string]any); ok {
			if message, ok := errObj["message"].(string); ok && message != "" {
				return message
			}
		}
		if message, ok := value["message"].(string); ok && message != "" {
			return message
		}
	}
	text := strings.TrimSpace(string(raw))
	if len(text) > 240 {
		text = text[:240]
	}
	if text == "" {
		return "空响应"
	}
	return text
}

func (e *moduleEnv) health(timeout time.Duration) (bool, map[string]any) {
	status, raw, err := e.request("/healthz", "", http.MethodGet, nil, timeout)
	if err != nil {
		return false, nil
	}
	value, err := decodeObject(raw)
	if err != nil {
		return false, nil
	}
	return status == http.StatusOK, value
}

func (e *moduleEnv) poolStatus() (map[string]any, error) {
	status, raw, err := e.request("/status", e.gatewayToken(), http.MethodGet, nil, 15*time.Second)
	if err != nil {
		return nil, fmt.Errorf("连接网关失败: %w", err)
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("读取账号池失败 HTTP %d: %s", status, apiError(raw))
	}
	return decodeObject(raw)
}

func (e *moduleEnv) adminRequest(path, method string, body any, timeout time.Duration) (int, []byte, error) {
	return e.request("/api/admin"+path, e.adminToken(), method, body, timeout)
}
