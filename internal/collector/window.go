package collector

import "time"

// PreviousSevenCompleteUTCDays returns the UTC start and exclusive end of the previous seven calendar days.
func PreviousSevenCompleteUTCDays(now time.Time) (time.Time, time.Time) {
	utcNow := now.UTC()
	end := time.Date(utcNow.Year(), utcNow.Month(), utcNow.Day(), 0, 0, 0, 0, time.UTC)
	return end.AddDate(0, 0, -7), end
}
