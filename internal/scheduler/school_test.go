package scheduler

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log"
	"os"
	"strings"
	"testing"
	"time"
)

type fakeTaskExecutor struct {
	schoolErr   error
	catErr      error
	schoolPanic bool
	catPanic    bool
	schoolN     int
	catN        int
	context     context.Context
}

func (f *fakeTaskExecutor) RunSchool(ctx context.Context, out io.Writer) error {
	f.schoolN++
	f.context = ctx
	if f.schoolPanic {
		panic("school exploded")
	}
	return f.schoolErr
}

func (f *fakeTaskExecutor) RunCat(ctx context.Context, out io.Writer) error {
	f.catN++
	f.context = ctx
	if f.catPanic {
		panic("cat exploded")
	}
	return f.catErr
}

// TestNextWakeSchoolSlot 开学季任务在 school_hours（默认 12 点）处有独立时点。
func TestNextWakeSchoolSlot(t *testing.T) {
	s := New(Config{
		CheckinHours:      []int{9},
		TravelDisabled:    true,
		ActivityDisabled:  true,
		KeepaliveDisabled: true,
		SchoolHours:       []int{12},
	})
	at, kinds := s.nextWake(time.Date(2026, 9, 14, 11, 0, 0, 0, time.Local))
	if want := time.Date(2026, 9, 14, 12, 0, 0, 0, time.Local); !at.Equal(want) {
		t.Errorf("next=%v want %v（school 12:00 独立时点）", at, want)
	}
	if len(kinds) != 1 || kinds[0] != taskSchool {
		t.Errorf("kinds=%v want [school]", kinds)
	}
}

// TestNextWakeCatSlot 夜猫子任务在 cat_hours（默认 1 点）处有独立时点。
func TestNextWakeCatSlot(t *testing.T) {
	s := New(Config{
		CheckinHours:      []int{9},
		TravelDisabled:    true,
		ActivityDisabled:  true,
		KeepaliveDisabled: true,
		CatHours:          []int{1},
	})
	at, kinds := s.nextWake(time.Date(2026, 9, 14, 23, 0, 0, 0, time.Local))
	if want := time.Date(2026, 9, 15, 1, 0, 0, 0, time.Local); !at.Equal(want) {
		t.Errorf("next=%v want %v（cat 01:00 独立时点）", at, want)
	}
	if len(kinds) != 1 || kinds[0] != taskCat {
		t.Errorf("kinds=%v want [cat]", kinds)
	}
}

// TestNextWakeSchoolCatDisabled 显式禁用 school/cat 后排程只剩签到时点。
func TestNextWakeSchoolCatDisabled(t *testing.T) {
	s := New(Config{
		CheckinHours:      []int{21},
		TravelDisabled:    true,
		ActivityDisabled:  true,
		KeepaliveDisabled: true,
		SchoolHours:       []int{12},
		CatHours:          []int{1},
		SchoolDisabled:    true,
		CatDisabled:       true,
	})
	at, kinds := s.nextWake(time.Date(2026, 9, 14, 8, 0, 0, 0, time.Local))
	if want := time.Date(2026, 9, 14, 21, 0, 0, 0, time.Local); !at.Equal(want) {
		t.Errorf("next=%v want %v（school/cat 禁用 → 只有签到 21:00）", at, want)
	}
	if len(kinds) != 1 || kinds[0] != taskCheckin {
		t.Errorf("kinds=%v want [checkin]", kinds)
	}
}

func TestRunSchoolAndCatUseInjectedExecutor(t *testing.T) {
	fake := &fakeTaskExecutor{}
	s := New(Config{TaskExecutor: fake})
	s.RunSchoolNow()
	s.RunCatNow()
	if fake.schoolN != 1 || fake.catN != 1 {
		t.Fatalf("calls school=%d cat=%d want 1/1", fake.schoolN, fake.catN)
	}
}

func TestDispatchSchoolCatFailureWarnsOnly(t *testing.T) {
	fake := &fakeTaskExecutor{
		schoolErr: errors.New("school boom"),
		catErr:    errors.New("cat boom"),
	}
	s := New(Config{TaskExecutor: fake})

	var buf bytes.Buffer
	log.SetOutput(&buf)
	log.SetFlags(0)
	t.Cleanup(func() {
		log.SetOutput(os.Stderr)
		log.SetFlags(log.LstdFlags)
	})

	s.dispatch(context.Background(), taskSchool)
	s.dispatch(context.Background(), taskCat)

	if fake.schoolN != 1 || fake.catN != 1 {
		t.Fatalf("calls school=%d cat=%d want 1/1", fake.schoolN, fake.catN)
	}
	out := buf.String()
	if !strings.Contains(out, "WARN: school: school boom") {
		t.Errorf("school 失败未按 WARN 记录:\n%s", out)
	}
	if !strings.Contains(out, "WARN: cat: cat boom") {
		t.Errorf("cat 失败未按 WARN 记录:\n%s", out)
	}
}

func TestDispatchPassesContext(t *testing.T) {
	fake := &fakeTaskExecutor{}
	s := New(Config{TaskExecutor: fake})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	s.dispatch(ctx, taskSchool)
	if fake.context != ctx {
		t.Fatal("dispatch did not pass scheduler context to task executor")
	}
}

func TestTaskExecutorPanicsBecomeWarnings(t *testing.T) {
	fake := &fakeTaskExecutor{schoolPanic: true, catPanic: true}
	s := New(Config{TaskExecutor: fake})
	var buf bytes.Buffer
	log.SetOutput(&buf)
	log.SetFlags(0)
	t.Cleanup(func() {
		log.SetOutput(os.Stderr)
		log.SetFlags(log.LstdFlags)
	})
	s.dispatch(context.Background(), taskSchool)
	s.dispatch(context.Background(), taskCat)
	out := buf.String()
	if !strings.Contains(out, "panic: school exploded") ||
		!strings.Contains(out, "panic: cat exploded") {
		t.Fatalf("panic was not converted to WARN:\n%s", out)
	}
}
