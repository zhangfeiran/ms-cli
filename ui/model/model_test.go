package model

import (
	"testing"
	"time"
)

func TestFormatWaitDurationMinutesAndSeconds(t *testing.T) {
	if got, want := FormatWaitDuration(65*time.Second), "01:05"; got != want {
		t.Fatalf("FormatWaitDuration(65s) = %q, want %q", got, want)
	}
}

func TestFormatWaitDurationHours(t *testing.T) {
	if got, want := FormatWaitDuration(time.Hour+2*time.Minute+3*time.Second), "01:02:03"; got != want {
		t.Fatalf("FormatWaitDuration(1h2m3s) = %q, want %q", got, want)
	}
}
