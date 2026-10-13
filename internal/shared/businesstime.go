package shared

import "time"

// BusinessLocation is the timezone used to decide what "today" is for HR
// rules (job changes, overtime, ...): Asia/Jakarta, as for the FX job. It is
// UTC when tzdata is unavailable.
var BusinessLocation = func() *time.Location {
	if loc, err := time.LoadLocation("Asia/Jakarta"); err == nil {
		return loc
	}
	return time.UTC
}()

// BusinessToday returns the current calendar date in BusinessLocation as a
// UTC-midnight date.
func BusinessToday() time.Time { return BusinessDate(time.Now()) }

// BusinessDate returns t's calendar date in BusinessLocation as a UTC-midnight
// date.
func BusinessDate(t time.Time) time.Time {
	y, m, d := t.In(BusinessLocation).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}
