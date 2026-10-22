package signin

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"

	"workbuddy2api/internal/auth"
	"workbuddy2api/internal/upstream"
)

func TestStatusClassification(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{"ok", nil, "OK"},
		{"already-code", &upstream.Error{Kind: upstream.ErrClient, Msg: `{"code":10001,"msg":"今天已签到"}`}, "ALREADY"},
		{"already-14001", &upstream.Error{Kind: upstream.ErrClient, Msg: "code=14001 msg=已签到"}, "ALREADY"},
		{"not-enabled", &upstream.Error{Kind: upstream.ErrClient, Msg: "功能未开启"}, "ALREADY"},
		{"plain-transport", errors.New("address already in use"), "FAIL"},
		{"server", &upstream.Error{Kind: upstream.ErrServer, Msg: "boom"}, "FAIL"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := CheckinStatusOf(tc.err); got != tc.want {
				t.Errorf("status=%s want %s", got, tc.want)
			}
		})
	}
}

func TestRefreshStatusOfWrappedSessionDead(t *testing.T) {
	original := &upstream.Error{Kind: upstream.ErrSessionDead, Status: 401, Msg: "12153"}
	wrapped := fmt.Errorf("outer: %w", original)
	if got := RefreshStatusOf(wrapped); got != "AUTH_INVALID" {
		t.Errorf("status=%s", got)
	}
	if got := RefreshStatusOf(errors.New("connection refused")); got != "FAIL" {
		t.Errorf("transport status=%s", got)
	}
}

type collectClient struct {
	refreshCalls int
	checkins     int
}

func (c *collectClient) RefreshToken(*auth.Auth) error {
	c.refreshCalls++
	return nil
}

func (c *collectClient) DailyCheckin(a *auth.Auth) error {
	c.checkins++
	if a.UID == "expired" {
		return &upstream.Error{Kind: upstream.ErrClient, Msg: "今天已签到"}
	}
	return nil
}

func (c *collectClient) UserResource(a *auth.Auth) (int64, error) {
	if a.UID == "expired" {
		return 0, errors.New("quota unavailable")
	}
	return 42, nil
}

func TestCollectReportsRowsCountsRefreshAndQuota(t *testing.T) {
	docs := map[string]string{
		"workbuddy-a.json": `{"auth":{"accessToken":"a","refreshToken":"r","expiresAt":4102444800},"account":{"uid":"fresh","nickname":"甲"}}`,
		"workbuddy-b.json": `{"auth":{"accessToken":"b","refreshToken":"r","expiresAt":1},"account":{"uid":"expired","nickname":"乙"}}`,
		"workbuddy-c.json": `{broken`,
	}
	saves := 0
	ioDeps := IO{
		LoadFiles: func(string) ([]string, error) {
			return []string{"workbuddy-a.json", "workbuddy-b.json", "workbuddy-c.json"}, nil
		},
		ReadFile: func(name string) ([]byte, error) {
			return []byte(docs[name]), nil
		},
		Parse:      auth.Parse,
		SaveAtomic: func(*auth.Auth) error { saves++; return nil },
		Backfill:   func(*auth.Auth) (bool, string) { return false, "cn" },
		Logf:       func(string, ...any) {},
	}
	client := &collectClient{}
	summary, err := CollectWithIO("unused", client, ioDeps)
	if err != nil {
		t.Fatal(err)
	}
	if len(summary.Rows) != 3 || summary.OK != 1 || summary.Already != 1 || summary.Fail != 1 {
		t.Fatalf("summary=%+v", summary)
	}
	if client.refreshCalls != 1 || client.checkins != 2 || saves != 1 {
		t.Errorf("refresh=%d checkins=%d saves=%d", client.refreshCalls, client.checkins, saves)
	}
	if summary.Rows[0].Remain != 42 || !summary.Rows[0].HasQuota {
		t.Errorf("fresh quota row=%+v", summary.Rows[0])
	}

	var out bytes.Buffer
	Render(&out, summary)
	for _, want := range []string{"total=3 ok=1 already=1 fail=1", "ALREADY", "LOAD_ERR"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("render missing %q:\n%s", want, out.String())
		}
	}
}
