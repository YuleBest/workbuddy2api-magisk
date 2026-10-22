//go:build !windows

package wbapicli

import (
	"io"
	"os"
	"os/exec"
	"os/user"
	"runtime"
	"strconv"
	"syscall"
	"time"
)

type defaultPlatform struct{}

func (defaultPlatform) GOOS() string { return runtime.GOOS }

func (defaultPlatform) ProcessAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || err == syscall.EPERM
}

func (defaultPlatform) Terminate(pid int) error {
	return syscall.Kill(pid, syscall.SIGTERM)
}

type unixProcess struct {
	process *os.Process
}

func (p unixProcess) PID() int                   { return p.process.Pid }
func (p unixProcess) Signal(sig os.Signal) error { return p.process.Signal(sig) }
func (p unixProcess) Release() error             { return p.process.Release() }

func (defaultPlatform) StartProcess(name string, args []string, dir string, stdout, stderr io.Writer) (ProcessHandle, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return unixProcess{process: cmd.Process}, nil
}

func (defaultPlatform) UserID(name string) (int, error) {
	u, err := user.Lookup(name)
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(u.Uid)
}

func (defaultPlatform) FileOwnerUID(name string) (int, error) {
	info, err := os.Stat(name)
	if err != nil {
		return 0, err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, os.ErrInvalid
	}
	return int(stat.Uid), nil
}

func (defaultPlatform) Readable(name string) bool {
	f, err := os.Open(name)
	if err != nil {
		return false
	}
	_ = f.Close()
	return true
}

func (defaultPlatform) Sleep(d time.Duration) { time.Sleep(d) }
