package ptr

import "time"

func Int(i int) *int {
	return &i
}

func Int64(i int64) *int64 {
	return &i
}

func String(s string) *string {
	return &s
}

func Duration(d time.Duration) *time.Duration {
	return &d
}
