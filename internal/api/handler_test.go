package api

import (
	"testing"
	"time"
)

func TestParseWindow(t *testing.T) {
	tests := []struct {
		value string
		want  time.Duration
		valid bool
	}{
		{"", time.Hour, true}, {"1h", time.Hour, true}, {"24h", 24 * time.Hour, true}, {"invalid", 0, false},
	}
	for _, test := range tests {
		got, err := parseWindow(test.value)
		if (err == nil) != test.valid || got != test.want {
			t.Fatalf("parseWindow(%q) = %v, %v", test.value, got, err)
		}
	}
}
