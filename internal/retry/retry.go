package retry

import (
	"context"
	"time"
)

func Run(ctx context.Context, attempts int, delay time.Duration, operation func() error) error {
	if attempts < 1 {
		attempts = 1
	}
	if delay <= 0 {
		delay = 250 * time.Millisecond
	}
	var last error
	for attempt := 0; attempt < attempts; attempt++ {
		if err := operation(); err == nil {
			return nil
		} else {
			last = err
		}
		if attempt == attempts-1 {
			break
		}
		wait := delay << attempt
		if wait > 5*time.Second {
			wait = 5 * time.Second
		}
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
	return last
}
