//go:build windows

package wbapicli

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"time"
)

type defaultPlatform struct{}

func (defaultPlatform) GOOS() string { return "windows" }

func (defaultPlatform) ProcessAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	defer p.Release()
	return true
}

func (defaultPlatform) Terminate(pid int) error {
	p, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	defer p.Release()
	return p.Kill()
}

type windowsProcess struct {
	process *os.Process
}

func (p windowsProcess) PID() int                   { return p.process.Pid }
func (p windowsProcess) Signal(sig os.Signal) error { return p.process.Signal(sig) }
func (p windowsProcess) Release() error             { return p.process.Release() }

func (defaultPlatform) StartProcess(name string, args []string, dir string, stdout, stderr io.Writer) (ProcessHandle, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return windowsProcess{process: cmd.Process}, nil
}

func (defaultPlatform) UserID(string) (int, error) {
	return 0, errors.New("uid lookup unsupported on windows")
}

func (defaultPlatform) FileOwnerUID(string) (int, error) {
	return 0, errors.New("file owner lookup unsupported on windows")
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
