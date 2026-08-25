package domain

import (
	"errors"
	"strings"
	"testing"
)

func TestFieldErrorWrapsInvalid(t *testing.T) {
	t.Parallel()
	err := FieldError{Field: "quantity", Message: "must be positive"}
	if !errors.Is(err, ErrInvalid) {
		t.Fatal("field error does not wrap invalid")
	}
	if err.Error() != "quantity: must be positive" {
		t.Fatalf("message=%q", err.Error())
	}
}

func TestStateErrorWrapsState(t *testing.T) {
	t.Parallel()
	err := StateError{Entity: "batch", From: "registered", To: "archived"}
	if !errors.Is(err, ErrState) {
		t.Fatal("state error does not wrap state")
	}
	for _, part := range []string{"batch", "registered", "archived"} {
		if !strings.Contains(err.Error(), part) {
			t.Fatalf("message %q missing %q", err.Error(), part)
		}
	}
}

func TestConflictErrorWrapsConflict(t *testing.T) {
	t.Parallel()
	err := ConflictError{Resource: "order", Reason: "duplicate idempotency key"}
	if !errors.Is(err, ErrConflict) {
		t.Fatal("conflict error does not wrap conflict")
	}
	if !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("message=%q", err.Error())
	}
}

func TestDependencyErrorRetainsBothClassifications(t *testing.T) {
	t.Parallel()
	cause := errors.New("broker unavailable")
	err := DependencyError{Operation: "publish settlement", Err: cause}
	if !errors.Is(err, ErrDependency) {
		t.Fatal("missing dependency classification")
	}
	if !errors.Is(err, cause) {
		t.Fatal("missing underlying cause")
	}
	if !strings.Contains(err.Error(), "publish settlement") {
		t.Fatalf("message=%q", err.Error())
	}
}

func TestDependencyErrorWithoutCause(t *testing.T) {
	t.Parallel()
	err := DependencyError{Operation: "unknown"}
	if !errors.Is(err, ErrDependency) {
		t.Fatal("missing dependency classification")
	}
}
