//go:build !windows && !aix && !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd && !solaris

package tasks

import "os"

type cronFileLock struct {
	file *os.File
}

func acquireCronLock(path string) (*cronFileLock, bool, error) {
	file, err := os.OpenFile(path+".exclusive", os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o644)
	if err != nil {
		if os.IsExist(err) {
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
	name := l.file.Name()
	l.file.Close()
	os.Remove(name)
}
