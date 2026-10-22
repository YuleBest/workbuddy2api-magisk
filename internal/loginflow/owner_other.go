//go:build !unix

package loginflow

import "fmt"

func fileOwnerUID(path string) (int, error) {
	return 0, fmt.Errorf("file owner uid is unavailable on this platform: %s", path)
}
