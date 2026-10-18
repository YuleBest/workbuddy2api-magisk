package timezone

import (
	"time"
	_ "time/tzdata"
)

// Init fixes the process timezone on Android, where Go otherwise defaults
// time.Local to UTC.
func Init() {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err == nil {
		time.Local = loc
	}
}
