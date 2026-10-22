// school.go 开学季任务与夜猫子任务调度：直接调用进程内 Go 任务系统。
package scheduler

import (
	"context"
	"fmt"
	"io"
	"log"

	"workbuddy2api/internal/tasks"
)

// TaskExecutor is the scheduler-facing task-system boundary. Tests inject this
// interface instead of launching an external interpreter or touching the network.
type TaskExecutor interface {
	RunSchool(context.Context, io.Writer) error
	RunCat(context.Context, io.Writer) error
}

type goTaskExecutor struct {
	service *tasks.Service
}

func newTaskExecutor() TaskExecutor {
	return &goTaskExecutor{service: tasks.NewService()}
}

func (e *goTaskExecutor) RunSchool(ctx context.Context, out io.Writer) error {
	stats := e.service.RunSchoolAll(ctx, out)
	if stats.Fail > 0 {
		return fmt.Errorf("fail=%d", stats.Fail)
	}
	return nil
}

func (e *goTaskExecutor) RunCat(ctx context.Context, out io.Writer) error {
	// task_runner currently always exits 0, including per-task failures.
	e.service.RunCatAll(ctx, out)
	return nil
}

func (s *Scheduler) runSchool(ctx context.Context) {
	defer func() {
		if recovered := recover(); recovered != nil {
			log.Printf("WARN: school: panic: %v", recovered)
		}
	}()
	executor := s.cfg.TaskExecutor
	if executor == nil {
		executor = newTaskExecutor()
	}
	if err := executor.RunSchool(ctx, io.Discard); err != nil {
		log.Printf("WARN: school: %v", err)
		return
	}
	log.Printf("school: ok")
}

func (s *Scheduler) runCat(ctx context.Context) {
	defer func() {
		if recovered := recover(); recovered != nil {
			log.Printf("WARN: cat: panic: %v", recovered)
		}
	}()
	executor := s.cfg.TaskExecutor
	if executor == nil {
		executor = newTaskExecutor()
	}
	if err := executor.RunCat(ctx, io.Discard); err != nil {
		log.Printf("WARN: cat: %v", err)
		return
	}
	log.Printf("cat: ok")
}

// RunSchoolNow 立即执行开学季全量任务。
func (s *Scheduler) RunSchoolNow() {
	s.runSchool(context.Background())
}

// RunCatNow 立即执行夜猫子任务。
func (s *Scheduler) RunCatNow() {
	s.runCat(context.Background())
}
