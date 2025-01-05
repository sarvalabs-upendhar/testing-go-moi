package common

import (
	"strings"
	"time"
)

// TimeFormat that works in filenames on windows.
var TimeFormat = strings.ReplaceAll(time.RFC3339, ":", "_")

// Since returns the duration since t.
func Since(t time.Time) time.Duration {
	return Now().Sub(t)
}

// Until returns the duration until t.
func Until(t time.Time) time.Duration {
	return t.Sub(Now())
}

// Now returns the current local time.
func Now() time.Time {
	return Canonical(time.Now())
}

// Canonical returns UTC time with no monotonic component.
// Stripping the monotonic component is for time equality.
func Canonical(t time.Time) time.Time {
	return t.Round(0).UTC()
}
