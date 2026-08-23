package common

import (
	"errors"
	"fmt"
)

var (
	ErrNotFound        = errors.New("not found")
	ErrConflict        = errors.New("conflict")
	ErrInvalid         = errors.New("invalid input")
	ErrForbidden       = errors.New("forbidden")
	ErrUnauthenticated = errors.New("unauthenticated")
	ErrExpired         = errors.New("expired")
	ErrCapacity        = errors.New("capacity exhausted")
	ErrState           = errors.New("invalid state transition")
	ErrDependency      = errors.New("dependency unavailable")
	ErrLeaseLost       = errors.New("worker lease lost")
)

type FieldError struct {
	Field   string
	Message string
}

func (e FieldError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

func (e FieldError) Unwrap() error { return ErrInvalid }

type StateError struct {
	Entity string
	ID     string
	From   string
	To     string
	Reason string
}

func (e StateError) Error() string {
	if e.Reason != "" {
		return fmt.Sprintf("%s %s cannot transition from %s to %s: %s", e.Entity, e.ID, e.From, e.To, e.Reason)
	}
	return fmt.Sprintf("%s %s cannot transition from %s to %s", e.Entity, e.ID, e.From, e.To)
}

func (e StateError) Unwrap() error { return ErrState }

type ConflictError struct {
	Entity   string
	ID       string
	Expected int64
	Actual   int64
}

func (e ConflictError) Error() string {
	return fmt.Sprintf("%s %s version conflict: expected %d, actual %d", e.Entity, e.ID, e.Expected, e.Actual)
}

func (e ConflictError) Unwrap() error { return ErrConflict }

type CapacityError struct {
	Resource  string
	Requested int
	Available int
}

func (e CapacityError) Error() string {
	return fmt.Sprintf("%s capacity is %d, requested %d", e.Resource, e.Available, e.Requested)
}

func (e CapacityError) Unwrap() error { return ErrCapacity }

func WrapDependency(operation string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w: %w", operation, ErrDependency, err)
}

func AuthenticationLookupError(err error) error {
	if err == nil {
		return nil
	}
	// A missing or otherwise unresolvable session record means the supplied
	// token is not a known credential. Report it as unauthenticated rather than
	// not_found so callers cannot treat an expired or bogus token as a missing
	// resource and retry it as a resource-absent error.
	return fmt.Errorf("session lookup failed: %w: %w", ErrUnauthenticated, err)
}
