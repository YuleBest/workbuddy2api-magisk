package main

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type serviceState struct {
	PID     int
	Running bool
}

func (e *moduleEnv) serviceState() serviceState {
	raw, err := os.ReadFile(e.PIDFile)
	if err != nil {
		return serviceState{}
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(raw)))
	if err != nil || pid <= 0 || !processAlive(pid) {
		return serviceState{}
	}
	cmdline, err := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid))
	if err != nil || !strings.Contains(string(cmdline), filepath.Base(e.ServerBin)) {
		return serviceState{}
	}
	return serviceState{PID: pid, Running: true}
}

func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	return syscall.Kill(pid, 0) == nil
}

func (e *moduleEnv) startService(quiet bool) error {
	if err := e.prepareRuntimeDirs(); err != nil {
		return err
	}
	if info, err := os.Stat(e.ServerBin); err != nil || !info.Mode().IsRegular() {
		return fmt.Errorf("服务端二进制不存在: %s", e.ServerBin)
	}
	if err := os.Chmod(e.ServerBin, 0o755); err != nil {
		return err
	}
	if state := e.serviceState(); state.Running {
		if e.waitReady(5 * time.Second) {
			if !quiet {
				fmt.Printf("网关已在运行 (PID %d)\n", state.PID)
			}
			return nil
		}
		return fmt.Errorf("进程存在但服务未就绪 (PID %d)，请查看 wbapi log", state.PID)
	}
	_ = os.Remove(e.PIDFile)
	e.rotateLog()

	logFile, err := os.OpenFile(e.LogFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer logFile.Close()
	if err := logFile.Chmod(0o600); err != nil {
		return err
	}
	fmt.Fprintf(logFile, "\n[%s] starting wb2api\n", time.Now().Format("2006-01-02 15:04:05"))

	cmd := exec.Command(e.ServerBin, "-config", e.ConfigPath)
	cmd.Dir = e.ModuleDir
	cmd.Env = environmentWith(map[string]string{
		"HOME":           e.ModuleDir,
		"TMPDIR":         e.TmpDir,
		"TZ":             "Asia/Shanghai",
		"WB2A_SCHEDULER": "0",
	})
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("启动服务端: %w", err)
	}
	pid := cmd.Process.Pid
	if err := writeFileAtomic(e.PIDFile, []byte(strconv.Itoa(pid)+"\n"), 0o600); err != nil {
		_ = cmd.Process.Kill()
		return err
	}
	_ = cmd.Process.Release()

	if !e.waitReady(15 * time.Second) {
		return fmt.Errorf("服务端未在 15 秒内就绪，查看: %s", e.LogFile)
	}
	if !quiet {
		fmt.Printf("网关已启动 (PID %d): %s\n", pid, e.baseURL())
	}
	return nil
}

func (e *moduleEnv) stopService(quiet bool) error {
	state := e.serviceState()
	if !state.Running {
		_ = os.Remove(e.PIDFile)
		if !quiet {
			fmt.Println("网关未运行")
		}
		return nil
	}
	if err := syscall.Kill(state.PID, syscall.SIGTERM); err != nil && err != syscall.ESRCH {
		return fmt.Errorf("停止网关: %w", err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if !processAlive(state.PID) {
			_ = os.Remove(e.PIDFile)
			if !quiet {
				fmt.Printf("网关已停止 (PID %d)\n", state.PID)
			}
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	if err := syscall.Kill(state.PID, syscall.SIGKILL); err != nil && err != syscall.ESRCH {
		return fmt.Errorf("强制停止网关: %w", err)
	}
	_ = os.Remove(e.PIDFile)
	if !quiet {
		fmt.Printf("网关已强制停止 (PID %d)\n", state.PID)
	}
	return nil
}

func (e *moduleEnv) restartService() error {
	if err := e.stopService(true); err != nil {
		return err
	}
	return e.startService(false)
}

func (e *moduleEnv) waitReady(timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		status, _, err := e.request("/healthz", "", http.MethodGet, nil, time.Second)
		if err == nil && status > 0 {
			return true
		}
		if !e.serviceState().Running {
			return false
		}
		time.Sleep(250 * time.Millisecond)
	}
	return false
}

func (e *moduleEnv) rotateLog() {
	info, err := os.Stat(e.LogFile)
	if err != nil || info.Size() < 4<<20 {
		return
	}
	_ = os.Remove(e.LogFile + ".1")
	_ = os.Rename(e.LogFile, e.LogFile+".1")
}

func environmentWith(values map[string]string) []string {
	current := os.Environ()
	out := make([]string, 0, len(current)+len(values))
	for _, entry := range current {
		key := entry
		if index := strings.IndexByte(entry, '='); index >= 0 {
			key = entry[:index]
		}
		if _, replace := values[key]; !replace {
			out = append(out, entry)
		}
	}
	for key, value := range values {
		out = append(out, key+"="+value)
	}
	return out
}

func runningText(running bool) string {
	if running {
		return "运行中"
	}
	return "未运行"
}
