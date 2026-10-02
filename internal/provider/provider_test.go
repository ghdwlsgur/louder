package provider

import (
	"testing"
	"time"
)

func TestNormalizeDailyWindow(t *testing.T) {
	utc := time.UTC
	plusOne := time.FixedZone("UTC+1", 60*60)
	tests := []struct {
		name      string
		start     time.Time
		end       time.Time
		wantValid bool
		wantStart time.Time
		wantEnd   time.Time
	}{
		{
			name:      "UTC midnight bounds",
			start:     time.Date(2026, time.October, 1, 0, 0, 0, 0, utc),
			end:       time.Date(2026, time.October, 3, 0, 0, 0, 0, utc),
			wantValid: true,
			wantStart: time.Date(2026, time.October, 1, 0, 0, 0, 0, utc),
			wantEnd:   time.Date(2026, time.October, 3, 0, 0, 0, 0, utc),
		},
		{
			name:      "offset-aware bounds representing UTC midnight",
			start:     time.Date(2026, time.October, 1, 1, 0, 0, 0, plusOne),
			end:       time.Date(2026, time.October, 2, 1, 0, 0, 0, plusOne),
			wantValid: true,
			wantStart: time.Date(2026, time.October, 1, 0, 0, 0, 0, utc),
			wantEnd:   time.Date(2026, time.October, 2, 0, 0, 0, 0, utc),
		},
		{
			name:  "empty interval",
			start: time.Date(2026, time.October, 1, 0, 0, 0, 0, utc),
			end:   time.Date(2026, time.October, 1, 0, 0, 0, 0, utc),
		},
		{
			name:  "reversed interval",
			start: time.Date(2026, time.October, 2, 0, 0, 0, 0, utc),
			end:   time.Date(2026, time.October, 1, 0, 0, 0, 0, utc),
		},
		{
			name:  "sub-day start",
			start: time.Date(2026, time.October, 1, 0, 0, 1, 0, utc),
			end:   time.Date(2026, time.October, 2, 0, 0, 0, 0, utc),
		},
		{
			name:  "sub-day end",
			start: time.Date(2026, time.October, 1, 0, 0, 0, 0, utc),
			end:   time.Date(2026, time.October, 2, 0, 0, 1, 0, utc),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			start, end, valid := NormalizeDailyWindow(test.start, test.end)
			if valid != test.wantValid {
				t.Fatalf("NormalizeDailyWindow() valid = %t, want %t", valid, test.wantValid)
			}
			if test.wantValid {
				if !start.Equal(test.wantStart) || start.Location() != time.UTC {
					t.Errorf("NormalizeDailyWindow() start = %v, want UTC %v", start, test.wantStart)
				}
				if !end.Equal(test.wantEnd) || end.Location() != time.UTC {
					t.Errorf("NormalizeDailyWindow() end = %v, want UTC %v", end, test.wantEnd)
				}
			}
		})
	}
}

func TestIsUTCMidnight(t *testing.T) {
	for _, test := range []struct {
		name  string
		value time.Time
		want  bool
	}{
		{
			name:  "offset-aware UTC midnight",
			value: time.Date(2026, time.October, 1, 1, 0, 0, 0, time.FixedZone("UTC+1", 60*60)),
			want:  true,
		},
		{
			name:  "sub-second UTC time",
			value: time.Date(2026, time.October, 1, 0, 0, 0, 1, time.UTC),
			want:  false,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := IsUTCMidnight(test.value); got != test.want {
				t.Fatalf("IsUTCMidnight() = %t, want %t", got, test.want)
			}
		})
	}
}

func TestProvidesDailyCostRecords(t *testing.T) {
	for _, test := range []struct {
		provider string
		want     bool
	}{
		{"aws", true}, {"azure", true}, {"gcp", true}, {"oci", true}, {"ibm", true}, {"alibaba", true},
		{"ncp", false}, {"nhn", false}, {"unknown", false},
	} {
		t.Run(test.provider, func(t *testing.T) {
			if got := ProvidesDailyCostRecords(test.provider); got != test.want {
				t.Fatalf("ProvidesDailyCostRecords(%q) = %t, want %t", test.provider, got, test.want)
			}
		})
	}
}
