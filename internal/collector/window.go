package collector

import "time"

// PreviousCompleteUTCDay returns the UTC start and exclusive end of the previous calendar day.
func PreviousCompleteUTCDay(now time.Time) (time.Time, time.Time) {
	utcNow := now.UTC()
	end := time.Date(utcNow.Year(), utcNow.Month(), utcNow.Day(), 0, 0, 0, 0, time.UTC)
	return end.AddDate(0, 0, -1), end
}
