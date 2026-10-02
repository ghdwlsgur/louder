package collector

import "time"

// PreviousEightCompleteUTCDays returns the UTC start and exclusive end of the previous eight calendar days.
func PreviousEightCompleteUTCDays(now time.Time) (time.Time, time.Time) {
	utcNow := now.UTC()
	end := time.Date(utcNow.Year(), utcNow.Month(), utcNow.Day(), 0, 0, 0, 0, time.UTC)
	return end.AddDate(0, 0, -8), end
}
