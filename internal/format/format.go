package format

import (
	"fmt"
	"time"
)

// Duration formats a duration as HH:MM:SS or MM:SS.
func Duration(d time.Duration) string {
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60
	if h > 0 {
		return fmt.Sprintf("%02d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%02d:%02d", m, s)
}

// DurationHuman formats a duration for human display.
// Examples: "2h", "30m", "1h30m", "45s"
func DurationHuman(d time.Duration) string {
	if d >= time.Hour {
		hours := d / time.Hour
		minutes := (d % time.Hour) / time.Minute
		if minutes > 0 {
			return fmt.Sprintf("%dh%dm", hours, minutes)
		}
		return fmt.Sprintf("%dh", hours)
	}
	if d >= time.Minute {
		return fmt.Sprintf("%dm", d/time.Minute)
	}
	return fmt.Sprintf("%ds", d/time.Second)
}

// Size formats a size in bytes for human display.
// Uses MB for sizes >= 1MB, KB otherwise.
func Size(bytes int64) string {
	const (
		kb = 1024
		mb = 1024 * kb
	)
	if bytes >= mb {
		return fmt.Sprintf("%d MB", bytes/mb)
	}
	if bytes >= kb {
		return fmt.Sprintf("%d KB", bytes/kb)
	}
	return fmt.Sprintf("%d bytes", bytes)
}
