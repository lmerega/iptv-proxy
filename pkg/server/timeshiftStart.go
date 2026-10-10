package server

import (
	"strings"
	"time"
)

// Some players append a twelve-hour clock marker to the timeshift path.
// Providers expect the same local time expressed with a twenty-four-hour clock.
func normalizeTimeshiftStart(start string) string {
	value := strings.ToUpper(strings.TrimSpace(start))
	for _, layout := range []string{"2006-01-02:03-04 PM", "2006-01-02:03:04 PM", "2006-01-02:03:04:05 PM"} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed.Format("2006-01-02:15-04")
		}
	}
	return start
}
