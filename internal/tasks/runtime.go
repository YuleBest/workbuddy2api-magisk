package tasks

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"time"
)

type Doer interface {
	Do(*http.Request) (*http.Response, error)
}

type Runtime struct {
	HTTP  Doer
	Now   func() time.Time
	Sleep func(context.Context, time.Duration) bool
	UUID  func() string
}

func DefaultRuntime() Runtime {
	client := &http.Client{}
	return Runtime{
		HTTP: client,
		Now:  time.Now,
		Sleep: func(ctx context.Context, d time.Duration) bool {
			if d <= 0 {
				return ctx.Err() == nil
			}
			timer := time.NewTimer(d)
			defer timer.Stop()
			select {
			case <-ctx.Done():
				return false
			case <-timer.C:
				return true
			}
		},
		UUID: newUUID,
	}
}

func (r Runtime) normalized() Runtime {
	d := DefaultRuntime()
	if r.HTTP != nil {
		d.HTTP = r.HTTP
	}
	if r.Now != nil {
		d.Now = r.Now
	}
	if r.Sleep != nil {
		d.Sleep = r.Sleep
	}
	if r.UUID != nil {
		d.UUID = r.UUID
	}
	return d
}

func (r Runtime) now() time.Time {
	return r.normalized().Now()
}

func (r Runtime) sleep(ctx context.Context, d time.Duration) bool {
	return r.normalized().Sleep(ctx, d)
}

func (r Runtime) millis() int64 {
	return r.now().UnixNano() / int64(time.Millisecond)
}

func newUUID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(fmt.Sprintf("crypto/rand: %v", err))
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	dst := make([]byte, 36)
	hex.Encode(dst[0:8], b[0:4])
	dst[8] = '-'
	hex.Encode(dst[9:13], b[4:6])
	dst[13] = '-'
	hex.Encode(dst[14:18], b[6:8])
	dst[18] = '-'
	hex.Encode(dst[19:23], b[8:10])
	dst[23] = '-'
	hex.Encode(dst[24:36], b[10:16])
	return string(dst)
}
