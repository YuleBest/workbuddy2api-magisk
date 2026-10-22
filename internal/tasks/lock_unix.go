//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package tasks

import (
	"os"
	"syscall"
)

type cronFileLock struct {
	file *os.File
}

func acquireCronLock(path string) (*cronFileLock, bool, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, false, err
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		file.Close()
		if err == syscall.EWOULDBLOCK || err == syscall.EAGAIN {
			return nil, false, nil
		}
		return nil, false, err
	}
	return &cronFileLock{file: file}, true, nil
}

func (l *cronFileLock) Close() {
	if l == nil || l.file == nil {
		return
	}
	syscall.Flock(int(l.file.Fd()), syscall.LOCK_UN)
	l.file.Close()
}
