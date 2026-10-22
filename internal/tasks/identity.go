package tasks

import (
	"crypto/md5"
	"encoding/hex"
)

func deriveID(a Account, salt string) string {
	sum := md5.Sum([]byte(salt + ":" + a.UID))
	id := hex.EncodeToString(sum[:])
	if len(id) > 36 {
		return id[:36]
	}
	return id
}
