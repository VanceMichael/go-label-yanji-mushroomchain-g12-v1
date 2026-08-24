package audit

import (
	"errors"
	"testing"
	"time"
)

func validEvent() Event {
	return Event{ID: "audit-1", TenantID: "tenant", ActorID: "actor", RequestID: "request", Action: "batch.inspect", Object: "batch", ObjectID: "batch-1", Outcome: OutcomeSucceeded, Metadata: []byte(`{"decision":"approved"}`), CreatedAt: time.Date(2026, 8, 25, 0, 0, 0, 0, time.UTC)}
}

func TestEventValidationAcceptsOutcomes(t *testing.T) {
	t.Parallel()
	for _, outcome := range []Outcome{OutcomeSucceeded, OutcomeRejected, OutcomeFailed} {
		event := validEvent()
		event.Outcome = outcome
		if err := event.Validate(); err != nil {
			t.Fatalf("outcome %s: %v", outcome, err)
		}
	}
}

func TestEventValidationRejectsRequiredFields(t *testing.T) {
	t.Parallel()
	tests := []func(*Event){func(v *Event) { v.ID = "" }, func(v *Event) { v.TenantID = "" }, func(v *Event) { v.ActorID = "" }, func(v *Event) { v.Action = "" }, func(v *Event) { v.Object = "" }, func(v *Event) { v.ObjectID = "" }, func(v *Event) { v.CreatedAt = time.Time{} }}
	for index, mutate := range tests {
		event := validEvent()
		mutate(&event)
		if err := event.Validate(); !errors.Is(err, ErrInvalidEvent) {
			t.Fatalf("case %d error=%v", index, err)
		}
	}
}

func TestEventValidationRejectsUnknownOutcome(t *testing.T) {
	t.Parallel()
	event := validEvent()
	event.Outcome = "maybe"
	if err := event.Validate(); !errors.Is(err, ErrInvalidEvent) {
		t.Fatalf("error=%v", err)
	}
}

func TestEventValidationRejectsMalformedMetadata(t *testing.T) {
	t.Parallel()
	event := validEvent()
	event.Metadata = []byte(`{"broken"`)
	if err := event.Validate(); !errors.Is(err, ErrInvalidEvent) {
		t.Fatalf("error=%v", err)
	}
}

func TestMetadataEncodesStructuredValue(t *testing.T) {
	t.Parallel()
	payload, err := Metadata(map[string]any{"quantity": 12, "released": true})
	if err != nil {
		t.Fatal(err)
	}
	event := validEvent()
	event.Metadata = payload
	if err = event.Validate(); err != nil {
		t.Fatalf("metadata did not validate: %v", err)
	}
}
