package tasks

import (
	"testing"
	"time"
)

func TestTaskMappingIsComplete(t *testing.T) {
	want := []string{
		"create_canvas", "template_5", "expert_5", "Expert_team_use_3", "skill_1",
		"automation_1", "playbook_prompt", "Expert_lighthouse", "Buddy_App", "Buddy_App_QQ",
		"Hp_Appearance", "chat_5", "Model_chat_GLM5.2", "black_cat", "first_buddy",
		"RichMeow_Chat", "Library_read", "chat_3_times", "expert_use", "share_invite",
		"desktop_chat_1_time", "Sequential_Tasks_1", "school_season", "Expert_Philanthropy",
	}
	for _, code := range want {
		if _, ok := specByCode(code); !ok {
			t.Errorf("mapping missing %s", code)
		}
	}
	if len(taskSpecs) != len(want) {
		t.Fatalf("mapping size=%d want %d", len(taskSpecs), len(want))
	}
}

func TestDeriveIDMatchesLegacyMD5Slice(t *testing.T) {
	account := testAccount()
	if got, want := deriveID(account, "machine"), "a4e974c60d0c8698e65c1c37145a21db"; got != want {
		t.Fatalf("machine id=%s want %s", got, want)
	}
	if got, want := deriveID(account, "session"), "5af592e227118519f866064decf3fa90"; got != want {
		t.Fatalf("session id=%s want %s", got, want)
	}
}

func TestNightWindowBoundaries(t *testing.T) {
	cst := time.FixedZone("CST", 8*60*60)
	for _, tc := range []struct {
		hour int
		want bool
	}{
		{hour: 22, want: false},
		{hour: 23, want: true},
		{hour: 0, want: true},
		{hour: 7, want: true},
		{hour: 8, want: false},
	} {
		clock := newFakeClock()
		clock.now = time.Date(2026, 10, 20, tc.hour, 0, 0, 0, cst)
		service := NewService()
		service.Runtime = clock.runtime(nil)
		if got := service.withinNightWindow(); got != tc.want {
			t.Errorf("hour=%d got=%t want=%t", tc.hour, got, tc.want)
		}
	}
}
