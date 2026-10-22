package wbapicli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMainHelpIsStructuredAndSkipsConfig(t *testing.T) {
	project := t.TempDir()
	if err := os.WriteFile(filepath.Join(project, "config.json"), []byte(`{broken`), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := Run(Options{
		Args:    []string{"help"},
		Env:     Env{},
		Project: project,
		Stdout:  &stdout,
		Stderr:  &stderr,
	})
	if code != 0 {
		t.Fatalf("help code=%d stderr=%s", code, stderr.String())
	}
	if stderr.Len() != 0 || strings.Contains(stdout.String(), "配置文件解析失败") {
		t.Fatalf("help loaded runtime config: stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
	for _, want := range []string{
		"wbapi - WorkBuddy2API 网关运维 CLI",
		"用法:",
		"服务:",
		"诊断与数据:",
		"账号与任务:",
		"wbapi help <命令>",
	} {
		if !strings.Contains(stdout.String(), want) {
			t.Errorf("main help missing %q:\n%s", want, stdout.String())
		}
	}
}

func TestCommandHelpVariants(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want []string
	}{
		{
			name: "command topic",
			args: []string{"help", "status"},
			want: []string{"wbapi status -", "wbapi check", "默认执行 status"},
		},
		{
			name: "help command with flag",
			args: []string{"help", "status", "--help"},
			want: []string{"wbapi status -", "wbapi check"},
		},
		{
			name: "nested topic",
			args: []string{"help", "acct", "enable"},
			want: []string{"wbapi acct enable -", "wbapi acct enable <uid|前缀>"},
		},
		{
			name: "flag after command",
			args: []string{"acct", "--help"},
			want: []string{"wbapi acct -", "wbapi acct disable <uid|前缀> [原因]"},
		},
		{
			name: "flag after subcommand",
			args: []string{"task", "checkin", "-h"},
			want: []string{"wbapi task checkin -", "wbapi task checkin [--async]"},
		},
		{
			name: "flag after option value",
			args: []string{"models", "--realm", "global", "--help"},
			want: []string{"wbapi models -", "--realm <cn|global>"},
		},
		{
			name: "subcommand alias",
			args: []string{"key", "rm", "--help"},
			want: []string{"wbapi key revoke -", "wbapi key revoke <前缀>"},
		},
		{
			name: "command alias",
			args: []string{"help", "pool"},
			want: []string{"wbapi quota -", "wbapi quota test [模型...]"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			code, stdout, stderr, _, _ := runTest(t, tc.args, "", nil)
			if code != 0 {
				t.Fatalf("code=%d stderr=%s", code, stderr)
			}
			for _, want := range tc.want {
				if !strings.Contains(stdout, want) {
					t.Errorf("help missing %q:\n%s", want, stdout)
				}
			}
		})
	}
}

func TestUnknownCommandsPointToHelp(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
	}{
		{"unknown command", []string{"wat"}},
		{"unknown topic", []string{"help", "wat"}},
		{"unknown subtopic", []string{"help", "acct", "wat"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			code, stdout, stderr, _, _ := runTest(t, tc.args, "", nil)
			if code != 2 {
				t.Fatalf("code=%d want 2", code)
			}
			if stdout != "" {
				t.Errorf("stdout=%q", stdout)
			}
			if !strings.Contains(stderr, "错误:") || !strings.Contains(stderr, "wbapi help") {
				t.Errorf("stderr=%q", stderr)
			}
		})
	}
}

func TestArgumentErrorsPointToSpecificHelp(t *testing.T) {
	tests := []struct {
		args []string
		want string
	}{
		{[]string{"acct", "disable"}, "wbapi help acct disable"},
		{[]string{"key", "revoke"}, "wbapi help key revoke"},
		{[]string{"log", "many"}, "wbapi help log"},
		{[]string{"task"}, "wbapi help task"},
	}
	for _, tc := range tests {
		t.Run(strings.Join(tc.args, "-"), func(t *testing.T) {
			code, _, stderr, _, _ := runTest(t, tc.args, "", nil)
			if code != 2 {
				t.Fatalf("code=%d stderr=%s", code, stderr)
			}
			if !strings.Contains(stderr, tc.want) {
				t.Errorf("stderr missing %q:\n%s", tc.want, stderr)
			}
		})
	}
}

func TestEveryHelpTopicHasContent(t *testing.T) {
	for topic, help := range commandHelpTopics {
		if help.Name == "" || help.Summary == "" {
			t.Errorf("topic %q has incomplete heading: %+v", topic, help)
		}
		if topic != "env" && topic != "auth" && len(help.Usage) == 0 {
			t.Errorf("topic %q has no usage", topic)
		}
	}
	for topic, children := range commandSubHelp {
		for sub, help := range children {
			if help.Name == "" || help.Summary == "" || len(help.Usage) == 0 {
				t.Errorf("subcommand %s %s is incomplete: %+v", topic, sub, help)
			}
		}
	}
}
