package tasks

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

const (
	growthUA  = "CLI/2.63.2 CodeBuddy/2.63.2"
	desktopUA = "WorkBuddy/5.5.6 WorkBuddy/5.5.6 CLI/2.137.1"
	webUA     = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 " +
		"(KHTML, like Gecko) Chrome/152.0.0.0 Safari/537.36"
	mpUA = "Mozilla/5.0 (Linux; Android 14; MicroMessenger/8.0.49 WeChat/0.8.0 " +
		"MiniProgramEnv/android; wkbrowser xweb)"
)

type httpResult struct {
	Status int
	Body   map[string]any
	Raw    string
}

func (s *Service) request(
	ctx context.Context,
	method, base, path string,
	body any,
	headers map[string]string,
	timeout time.Duration,
) (httpResult, error) {
	s.normalize()
	url := path
	if !strings.HasPrefix(path, "http://") && !strings.HasPrefix(path, "https://") {
		url = strings.TrimRight(base, "/") + path
	}
	var payload io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return httpResult{Status: -1}, err
		}
		payload = bytes.NewReader(raw)
	}
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, method, url, payload)
	if err != nil {
		return httpResult{Status: -1}, err
	}
	for key, value := range headers {
		req.Header[key] = []string{value}
	}
	if req.Header.Get("Accept-Encoding") == "" {
		req.Header.Set("Accept-Encoding", "identity")
	}
	if req.Header.Get("Connection") == "" {
		req.Header.Set("Connection", "close")
	}
	resp, err := s.Runtime.HTTP.Do(req)
	if err != nil {
		return httpResult{Status: -1}, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return httpResult{Status: resp.StatusCode}, err
	}
	out := httpResult{Status: resp.StatusCode, Raw: string(raw)}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&out.Body); err != nil {
		text := strings.ToValidUTF8(string(raw), "\uFFFD")
		if len(text) > 300 {
			text = text[:300]
		}
		out.Body = map[string]any{"raw": text}
	}
	return out, nil
}

func growthHeaders(a Account) map[string]string {
	h := map[string]string{
		"Authorization": "Bearer " + a.Token,
		"Accept":        "application/json",
		"Content-Type":  "application/json",
		"User-Agent":    growthUA,
		"Origin":        "https://www.codebuddy.cn",
		"Referer":       "https://www.codebuddy.cn/",
	}
	if a.UID != "" {
		h["X-User-Id"] = a.UID
	}
	if a.Domain != "" {
		h["X-Domain"] = a.Domain
	}
	return h
}

func mergeHeaders(base, extra map[string]string) map[string]string {
	out := make(map[string]string, len(base)+len(extra))
	for key, value := range base {
		out[key] = value
	}
	for key, value := range extra {
		out[key] = value
	}
	return out
}

func (s *Service) doGrowth(
	ctx context.Context,
	a Account,
	method, base, path string,
	body any,
	extra map[string]string,
) (int, map[string]any) {
	res, err := s.request(ctx, method, base, path, body, mergeHeaders(growthHeaders(a), extra), 30*time.Second)
	if err != nil {
		return -1, map[string]any{"err": err.Error()}
	}
	return res.Status, res.Body
}

func codeOf(body map[string]any) any {
	if body == nil {
		return nil
	}
	return body["code"]
}

func number(v any) float64 {
	n, _ := numericNumber(v)
	return n
}

func numericNumber(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case json.Number:
		f, _ := n.Float64()
		return f, true
	default:
		return 0, false
	}
}

func truthy(v any) bool {
	switch x := v.(type) {
	case nil:
		return false
	case bool:
		return x
	case string:
		return x != ""
	case float64:
		return x != 0
	case float32:
		return x != 0
	case int:
		return x != 0
	case int64:
		return x != 0
	case json.Number:
		n, _ := x.Float64()
		return n != 0
	case map[string]any:
		return len(x) != 0
	case []any:
		return len(x) != 0
	default:
		return true
	}
}

func stringValue(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case json.Number:
		return x.String()
	case float64:
		return fmt.Sprintf("%v", x)
	case nil:
		return ""
	default:
		return fmt.Sprint(x)
	}
}

func legacyValue(v any) string {
	switch x := v.(type) {
	case nil:
		return "None"
	case bool:
		if x {
			return "True"
		}
		return "False"
	case float64:
		return formatFloat(x)
	case float32:
		return formatFloat(float64(x))
	default:
		return stringValue(v)
	}
}

func object(v any) map[string]any {
	m, _ := v.(map[string]any)
	return m
}

func array(v any) []any {
	a, _ := v.([]any)
	return a
}

func isOK(body map[string]any) bool {
	code := codeOf(body)
	n, ok := numericNumber(code)
	return ok && n == 0
}

func codeZeroOrNil(v any) bool {
	if v == nil {
		return true
	}
	n, ok := numericNumber(v)
	return ok && n == 0
}
