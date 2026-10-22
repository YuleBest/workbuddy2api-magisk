//go:build windows

package tasks

import (
	"os"
	"syscall"
	"unsafe"
)

const (
	lockfileFailImmediately = 0x00000001
	lockfileExclusiveLock   = 0x00000002
)

var (
	kernel32         = syscall.NewLazyDLL("kernel32.dll")
	procLockFileEx   = kernel32.NewProc("LockFileEx")
	procUnlockFileEx = kernel32.NewProc("UnlockFileEx")
)

type cronFileLock struct {
	file     *os.File
	overlap  syscall.Overlapped
	unlocked bool
}

func acquireCronLock(path string) (*cronFileLock, bool, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, false, err
	}
	lock := &cronFileLock{file: file}
	ok, _, callErr := procLockFileEx.Call(
		file.Fd(),
		lockfileExclusiveLock|lockfileFailImmediately,
		0,
		1,
		0,
		uintptr(unsafe.Pointer(&lock.overlap)),
	)
	if ok == 0 {
		file.Close()
		if errno, ok := callErr.(syscall.Errno); ok && (errno == 32 || errno == 33) {
			return nil, false, nil
		}
		return nil, false, callErr
	}
	return lock, true, nil
}

func (l *cronFileLock) Close() {
	if l == nil || l.file == nil || l.unlocked {
		return
	}
	l.unlocked = true
	procUnlockFileEx.Call(
		l.file.Fd(),
		0,
		1,
		0,
		uintptr(unsafe.Pointer(&l.overlap)),
	)
	l.file.Close()
}
