package domain

import (
	"errors"
	"fmt"
)

var (
	ErrNotFound        = errors.New("not found")
	ErrConflict        = errors.New("conflict")
	ErrInvalid         = errors.New("invalid input")
	ErrUnauthorized    = errors.New("unauthorized")
	ErrForbidden       = errors.New("forbidden")
	ErrExpired         = errors.New("expired")
	ErrVersionConflict = errors.New("version conflict")
	ErrCapacity        = errors.New("capacity unavailable")
	ErrState           = errors.New("invalid state transition")
	ErrDependency      = errors.New("dependency failure")
)

type FieldError struct {
	Field   string
	Message string
}

func (e FieldError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

func (e FieldError) Unwrap() error {
	return ErrInvalid
}

type StateError struct {
	Entity string
	From   string
	To     string
}

func (e StateError) Error() string {
	return fmt.Sprintf("%s cannot transition from %s to %s", e.Entity, e.From, e.To)
}

func (e StateError) Unwrap() error {
	return ErrState
}

type ConflictError struct {
	Resource string
	Reason   string
}

func (e ConflictError) Error() string {
	return fmt.Sprintf("%s conflict: %s", e.Resource, e.Reason)
}

func (e ConflictError) Unwrap() error {
	return ErrConflict
}

type DependencyError struct {
	Operation string
	Err       error
}

func (e DependencyError) Error() string {
	return fmt.Sprintf("%s: %v", e.Operation, e.Err)
}

func (e DependencyError) Unwrap() error {
	if e.Err == nil {
		return ErrDependency
	}
	return errors.Join(ErrDependency, e.Err)
}
