package schedule

import (
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestValidateInterval(t *testing.T) {
	starts := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	before := starts.Add(-time.Second)
	equal := starts
	after := starts.Add(time.Second)

	for name, ends := range map[string]*time.Time{
		"before": &before,
		"equal":  &equal,
		"open":   nil,
		"after":  &after,
	} {
		t.Run(name, func(t *testing.T) {
			err := validateInterval(starts, ends)
			if name == "before" || name == "equal" {
				if !errors.Is(err, ErrInvalidInterval) {
					t.Fatalf("error=%v, want ErrInvalidInterval", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestMapScheduleWriteError(t *testing.T) {
	err := mapScheduleWriteError(&pgconn.PgError{Code: "23P01", ConstraintName: "schedule_items_no_overlap"})
	if !errors.Is(err, ErrScheduleConflict) {
		t.Fatalf("error=%v, want ErrScheduleConflict", err)
	}

	original := errors.New("database unavailable")
	if got := mapScheduleWriteError(original); !errors.Is(got, original) {
		t.Fatalf("error=%v, want original error", got)
	}
}
