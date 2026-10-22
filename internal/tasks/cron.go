package tasks

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const (
	cronLogLimit  = 2_000_000
	cronTailLines = 2000
)

func FindRepoRoot() string {
	cwd, err := os.Getwd()
	if err != nil {
		return "."
	}
	dir := cwd
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return cwd
		}
		dir = parent
	}
}

func TaskCron(
	ctx context.Context,
	args []string,
	stdout, stderr io.Writer,
	service *Service,
	root string,
) int {
	if service == nil {
		service = NewService()
	}
	if len(args) != 1 || (args[0] != "school" && args[0] != "cat") {
		fmt.Fprintln(stderr, "用法: taskcron {school|cat}")
		fmt.Fprintln(stderr, "  school  开学任务：任务+claim+自动抽空抽奖")
		fmt.Fprintln(stderr, "  cat     夜猫窗口(23-08 CST)补 1 次")
		return 2
	}
	mode := args[0]
	if root == "" {
		root = FindRepoRoot()
	}
	dataDir := filepath.Join(root, "data")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		fmt.Fprintf(stderr, "ERROR: 创建 data 目录失败: %v\n", err)
		return 1
	}
	logPath := filepath.Join(dataDir, "school-cron.log")
	lockPath := filepath.Join(dataDir, "school-cron.lock")
	lock, acquired, err := acquireCronLock(lockPath)
	if err != nil {
		fmt.Fprintf(stderr, "ERROR: 获取 cron 锁失败: %v\n", err)
		return 1
	}
	now := service.Runtime.normalized().Now()
	if !acquired {
		appendCronLine(logPath, fmt.Sprintf("[%s] skip[%s]: 上一次运行仍持有锁，跳过本次",
			now.Format("2006-01-02 15:04:05"), mode))
		return 0
	}
	defer lock.Close()

	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		fmt.Fprintf(stderr, "ERROR: 打开 cron 日志失败: %v\n", err)
		return 1
	}
	command := "school_open_day_2026 ALL --run --yes"
	if mode == "cat" {
		command = "task_runner ALL --yes --only black_cat"
	}
	start := now.Format("2006-01-02 15:04:05")
	fmt.Fprintf(logFile, "[%s] === cron[%s] start: %s ===\n", start, mode, command)

	var exitCode int
	var panicValue any
	if mode == "school" {
		exitCode, panicValue = runCronCLI(func() int {
			return SchoolCLIContext(ctx, []string{"ALL", "--run", "--yes"}, logFile, logFile, service)
		})
	} else {
		exitCode, panicValue = runCronCLI(func() int {
			return TaskRunnerCLIContext(ctx, []string{"ALL", "--yes", "--only", "black_cat"}, logFile, logFile, service)
		})
	}
	endNow := service.Runtime.normalized().Now()
	end := endNow.Format("2006-01-02 15:04:05")
	if panicValue != nil {
		fmt.Fprintf(logFile, "[%s] ERROR[%s]: panic: %v\n", end, mode, panicValue)
		fmt.Fprintf(logFile, "[%s] === cron[%s] end: 失败 ec=1 ===\n", end, mode)
	} else if exitCode != 0 {
		fmt.Fprintf(logFile, "[%s] ERROR[%s]: 退出码 %d（详见上方输出）\n", end, mode, exitCode)
		fmt.Fprintf(logFile, "[%s] === cron[%s] end: 失败 ec=%d ===\n", end, mode, exitCode)
	} else {
		fmt.Fprintf(logFile, "[%s] === cron[%s] end: 完成 ===\n", end, mode)
	}
	if err := logFile.Close(); err != nil {
		fmt.Fprintf(stderr, "ERROR: 关闭 cron 日志失败: %v\n", err)
		return 1
	}
	if err := rotateCronLog(logPath); err != nil {
		fmt.Fprintf(stderr, "ERROR: cron 日志轮转失败: %v\n", err)
		return 1
	}
	return 0
}

func runCronCLI(run func() int) (code int, panicValue any) {
	defer func() {
		panicValue = recover()
	}()
	return run(), nil
}

func appendCronLine(path, line string) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return
	}
	defer file.Close()
	fmt.Fprintln(file, line)
}

func rotateCronLog(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.Size() <= cronLogLimit {
		return nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	trimmed := tailLogLines(raw, cronTailLines)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, trimmed, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func tailLogLines(raw []byte, keep int) []byte {
	lines := bytes.SplitAfter(raw, []byte{'\n'})
	if len(lines) > 0 && len(lines[len(lines)-1]) == 0 {
		lines = lines[:len(lines)-1]
	}
	if len(lines) > keep {
		lines = lines[len(lines)-keep:]
	}
	return bytes.Join(lines, nil)
}
