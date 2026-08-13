package misc

import (
	"fmt"
	"time"
)

// ConvertTimestamp parses a Unix timestamp or a common date format into a
// time.Time. An empty input returns the current time. Recognized date formats
// are RFC3339, ISO-like layouts, and RFC1123.
func ConvertTimestamp(input string) (time.Time, error) {
	if input == "" {
		return time.Now(), nil
	}
	formats := []string{
		time.RFC3339,
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05",
		"2006-01-02",
		"2006/01/02",
		time.RFC1123,
	}
	for _, f := range formats {
		if t, err := time.Parse(f, input); err == nil {
			return t, nil
		}
	}
	// Only treat the input as a Unix timestamp if it is a pure number.
	var sec int64
	if _, err := fmt.Sscanf(input, "%d", &sec); err == nil && fmt.Sprint(sec) == input {
		return time.Unix(sec, 0), nil
	}
	return time.Time{}, fmt.Errorf("unable to parse time: %s", input)
}
