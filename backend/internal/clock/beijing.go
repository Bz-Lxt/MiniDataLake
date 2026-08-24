package clock

import (
	"time"
)

// Beijing is GMT+8 without DST.
var Beijing = time.FixedZone("CST", 8*60*60)

func Now() time.Time {
	return time.Now().In(Beijing)
}

func Format(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.In(Beijing).Format("2006-01-02 15:04:05")
}

func ToBeijing(t time.Time) time.Time {
	if t.IsZero() {
		return t
	}
	return t.In(Beijing)
}
