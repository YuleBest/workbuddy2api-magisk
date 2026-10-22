//go:build unix

package loginflow

import (
	"fmt"
	"os"
	"syscall"
)

func fileOwnerUID(path string) (int, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, fmt.Errorf("unsupported stat owner for %s", path)
	}
	return int(stat.Uid), nil
}
