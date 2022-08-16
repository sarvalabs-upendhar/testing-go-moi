package kutils

import "time"

// Now returns the current time in UTC with no monotonic component.
func Now() time.Time {
	return Canonical(time.Now())
}

// Canonical returns UTC time with no monotonic component.
// Stripping the monotonic component is for time equality.
func Canonical(t time.Time) time.Time {
	return t.Round(0).UTC()
}
