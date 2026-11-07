package playout

import (
	"testing"
	"time"
)

func TestPositionIsNonNegative(t *testing.T) {
	started := time.Now().UTC().Add(-3 * time.Second)
	position := time.Since(started).Milliseconds()
	if position < 0 {
		t.Fatal("playout position cannot be negative")
	}
}
