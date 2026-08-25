package audit

import (
	"encoding/json"
	"errors"
	"strings"
	"time"
)

var ErrInvalidEvent = errors.New("invalid audit event")

type Outcome string

const (
	OutcomeSucceeded Outcome = "succeeded"
	OutcomeRejected  Outcome = "rejected"
	OutcomeFailed    Outcome = "failed"
)

type Event struct {
	ID        string          `json:"id"`
	TenantID  string          `json:"tenant_id"`
	ActorID   string          `json:"actor_id"`
	RequestID string          `json:"request_id"`
	Action    string          `json:"action"`
	Object    string          `json:"object"`
	ObjectID  string          `json:"object_id"`
	Outcome   Outcome         `json:"outcome"`
	Metadata  json.RawMessage `json:"metadata,omitempty"`
	CreatedAt time.Time       `json:"created_at"`
}

func (e Event) Validate() error {
	if strings.TrimSpace(e.ID) == "" || strings.TrimSpace(e.TenantID) == "" ||
		strings.TrimSpace(e.ActorID) == "" || strings.TrimSpace(e.Action) == "" ||
		strings.TrimSpace(e.Object) == "" || strings.TrimSpace(e.ObjectID) == "" {
		return ErrInvalidEvent
	}
	switch e.Outcome {
	case OutcomeSucceeded, OutcomeRejected, OutcomeFailed:
	default:
		return ErrInvalidEvent
	}
	if e.CreatedAt.IsZero() {
		return ErrInvalidEvent
	}
	if len(e.Metadata) > 0 && !json.Valid(e.Metadata) {
		return ErrInvalidEvent
	}
	return nil
}

func Metadata(value any) (json.RawMessage, error) {
	payload, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return payload, nil
}
