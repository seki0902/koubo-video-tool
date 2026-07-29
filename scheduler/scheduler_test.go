package scheduler

import (
	"testing"
	"time"
)

func TestNextInterval(t *testing.T) {
	tests := []struct {
		count    int
		expected time.Duration
	}{
		{0, 2 * time.Second},
		{1, 2 * time.Second},
		{2, 4 * time.Second},
		{3, 8 * time.Second},
		{4, 16 * time.Second},
		{5, 30 * time.Second},
		{10, 30 * time.Second},
	}

	for _, tt := range tests {
		got := nextInterval(tt.count)
		if got != tt.expected {
			t.Errorf("nextInterval(%d) = %v, want %v", tt.count, got, tt.expected)
		}
	}
}

func TestMaxPolls(t *testing.T) {
	if maxPolls != 30 {
		t.Errorf("maxPolls = %d, want 30", maxPolls)
	}
}
