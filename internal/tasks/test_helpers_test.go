package tasks

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

type recordedRequest struct {
	Method string
	Path   string
	Header http.Header
	Body   []byte
}

type requestLog struct {
	mu       sync.Mutex
	requests []recordedRequest
}

func (l *requestLog) add(r *http.Request) []byte {
	raw, _ := io.ReadAll(r.Body)
	entry := recordedRequest{
		Method: r.Method,
		Path:   r.URL.Path,
		Header: r.Header.Clone(),
		Body:   raw,
	}
	l.mu.Lock()
	l.requests = append(l.requests, entry)
	l.mu.Unlock()
	return raw
}

func (l *requestLog) snapshot() []recordedRequest {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]recordedRequest, len(l.requests))
	copy(out, l.requests)
	return out
}

type fakeClock struct {
	now    time.Time
	sleeps []time.Duration
	uuids  []string
	uuidN  int
}

func newFakeClock() *fakeClock {
	return &fakeClock{now: time.Date(2026, 10, 20, 12, 0, 0, 0, time.FixedZone("CST", 8*60*60))}
}

func (f *fakeClock) runtime(client Doer) Runtime {
	return Runtime{
		HTTP: client,
		Now:  func() time.Time { return f.now },
		Sleep: func(ctx context.Context, d time.Duration) bool {
			f.sleeps = append(f.sleeps, d)
			return ctx.Err() == nil
		},
		UUID: func() string {
			f.uuidN++
			if len(f.uuids) >= f.uuidN {
				return f.uuids[f.uuidN-1]
			}
			return "00000000-0000-4000-8000-" + string(rune('0'+f.uuidN))
		},
	}
}

func testService(t *testing.T, handler http.Handler, clock *fakeClock) (*Service, *httptest.Server) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	if clock == nil {
		clock = newFakeClock()
	}
	service := NewService()
	service.Runtime = clock.runtime(server.Client())
	service.ChatBase = server.URL + "/chat"
	service.BillingBase = server.URL + "/billing"
	service.CloudAgent = server.URL + "/school"
	service.Copilot = server.URL + "/copilot"
	service.WebBase = server.URL + "/web"
	service.COSExpert = server.URL + "/cos/expert.json"
	service.Playbook = server.URL + "/playbook"
	service.AuthsDir = t.TempDir()
	return service, server
}

func testAccount() Account {
	return Account{
		Token:    "token-12345678",
		UID:      "uid-1234567890",
		Nickname: "测试账号",
		Domain:   "copilot.tencent.com",
		Realm:    "cn",
		File:     "workbuddy-uid-1234.json",
	}
}

func writeJSONResponse(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeAuthFile(t *testing.T, dir, name string, doc map[string]any) string {
	t.Helper()
	raw, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func authDocument(uid, token, domain, realm string) map[string]any {
	authPart := map[string]any{
		"accessToken":  token,
		"refreshToken": "refresh",
		"expiresAt":    1999999999,
		"domain":       domain,
	}
	if realm != "" {
		authPart["realm"] = realm
	}
	return map[string]any{
		"auth": authPart,
		"account": map[string]any{
			"uid": uid, "nickname": "测试",
		},
	}
}

func decodeBody(t *testing.T, raw []byte) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("decode body: %v\n%s", err, raw)
	}
	return body
}

func decodeEventArray(t *testing.T, raw []byte) []map[string]any {
	t.Helper()
	var events []map[string]any
	if err := json.Unmarshal(raw, &events); err != nil {
		t.Fatalf("decode events: %v\n%s", err, raw)
	}
	return events
}

func jsonBytes(v any) []byte {
	raw, _ := json.Marshal(v)
	return raw
}

func bodyContains(raw []byte, fragments ...string) bool {
	for _, fragment := range fragments {
		if !bytes.Contains(raw, []byte(fragment)) {
			return false
		}
	}
	return true
}
