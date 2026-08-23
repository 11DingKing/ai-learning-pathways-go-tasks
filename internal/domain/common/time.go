package common

import (
	"fmt"
	"time"
)

type Clock interface {
	Now() time.Time
}

type SystemClock struct{}

func (SystemClock) Now() time.Time { return time.Now().UTC() }

type FixedClock struct {
	Value time.Time
}

func (c FixedClock) Now() time.Time { return c.Value.UTC() }

type Window struct {
	StartsAt time.Time
	EndsAt   time.Time
}

func NewWindow(startsAt, endsAt time.Time) (Window, error) {
	startsAt = startsAt.UTC()
	endsAt = endsAt.UTC()
	if startsAt.IsZero() || endsAt.IsZero() {
		return Window{}, FieldError{Field: "window", Message: "start and end are required"}
	}
	if !endsAt.After(startsAt) {
		return Window{}, FieldError{Field: "window", Message: "end must be after start"}
	}
	return Window{StartsAt: startsAt, EndsAt: endsAt}, nil
}

func (w Window) Contains(value time.Time) bool {
	value = value.UTC()
	return !value.Before(w.StartsAt) && value.Before(w.EndsAt)
}

func (w Window) Overlaps(other Window) bool {
	return w.StartsAt.Before(other.EndsAt) && other.StartsAt.Before(w.EndsAt)
}

func (w Window) String() string {
	return fmt.Sprintf("[%s,%s)", w.StartsAt.Format(time.RFC3339), w.EndsAt.Format(time.RFC3339))
}
