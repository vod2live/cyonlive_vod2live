package retry

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestRunRetries(t *testing.T) {
	n := 0
	err := Run(context.Background(), 3, time.Millisecond, func() error {
		n++
		if n < 3 {
			return errors.New("temporary")
		}
		return nil
	})
	if err != nil || n != 3 {
		t.Fatalf("err=%v attempts=%d", err, n)
	}
}
