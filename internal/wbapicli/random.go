package wbapicli

import (
	"crypto/rand"
	"io"
)

type cryptoRandomReader struct{}

func (cryptoRandomReader) Read(p []byte) (int, error) {
	return rand.Read(p)
}

var _ io.Reader = cryptoRandomReader{}
