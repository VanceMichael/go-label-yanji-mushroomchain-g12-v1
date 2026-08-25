package domain

import (
	"errors"
	"testing"
	"time"
)

func validBatch() SubstrateBatch {
	now := time.Date(2026, 8, 25, 0, 0, 0, 0, time.UTC)
	return SubstrateBatch{ID: "batch", TenantID: "tenant", FarmID: "farm", Code: "code", Species: "oyster", ProducedAt: now, ExpiresAt: now.Add(24 * time.Hour), QuantityProduced: 100, QuantityAvailable: 100, UnitPriceCents: 300, Status: BatchRegistered, Version: 1, CreatedAt: now, UpdatedAt: now}
}

func TestBatchValidationAcceptsCompleteBatch(t *testing.T) {
	t.Parallel()
	if err := validBatch().Validate(); err != nil {
		t.Fatalf("valid batch: %v", err)
	}
}

func TestBatchValidationRejectsMissingIdentity(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		mutate func(*SubstrateBatch)
	}{{"id", func(v *SubstrateBatch) { v.ID = "" }}, {"tenant", func(v *SubstrateBatch) { v.TenantID = "" }}, {"farm", func(v *SubstrateBatch) { v.FarmID = "" }}, {"code", func(v *SubstrateBatch) { v.Code = "" }}}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value := validBatch()
			test.mutate(&value)
			if err := value.Validate(); !errors.Is(err, ErrInvalid) {
				t.Fatalf("Validate error=%v", err)
			}
		})
	}
}

func TestBatchValidationRejectsQuantityViolations(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name                string
		produced, available int64
	}{{"zero produced", 0, 0}, {"negative produced", -1, 0}, {"negative available", 10, -1}, {"more available than produced", 10, 11}}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value := validBatch()
			value.QuantityProduced = test.produced
			value.QuantityAvailable = test.available
			if err := value.Validate(); !errors.Is(err, ErrInvalid) {
				t.Fatalf("Validate error=%v", err)
			}
		})
	}
}

func TestBatchValidationRejectsPriceAndDates(t *testing.T) {
	t.Parallel()
	value := validBatch()
	value.UnitPriceCents = 0
	if err := value.Validate(); !errors.Is(err, ErrInvalid) {
		t.Fatalf("price error=%v", err)
	}
	value = validBatch()
	value.ExpiresAt = value.ProducedAt
	if err := value.Validate(); !errors.Is(err, ErrInvalid) {
		t.Fatalf("date error=%v", err)
	}
	value.ExpiresAt = value.ProducedAt.Add(-time.Second)
	if err := value.Validate(); !errors.Is(err, ErrInvalid) {
		t.Fatalf("past expiry error=%v", err)
	}
}

func validOrder() Order {
	now := time.Date(2026, 8, 25, 0, 0, 0, 0, time.UTC)
	return Order{ID: "order", TenantID: "tenant", BuyerName: "buyer", DeliveryRegion: "region", Status: OrderDraft, IdempotencyKey: "key", Version: 1, RequestedAt: now, DueAt: now.Add(time.Hour), CreatedAt: now, UpdatedAt: now, Lines: []OrderLine{{ID: "line", OrderID: "order", Species: "oyster", Quantity: 10, UnitPriceCents: 300}}}
}

func TestOrderValidationAcceptsCompleteOrder(t *testing.T) {
	t.Parallel()
	if err := validOrder().Validate(); err != nil {
		t.Fatalf("valid order: %v", err)
	}
}

func TestOrderValidationRejectsIdentityAndBuyer(t *testing.T) {
	t.Parallel()
	tests := []func(*Order){func(v *Order) { v.ID = "" }, func(v *Order) { v.TenantID = "" }, func(v *Order) { v.BuyerName = "" }, func(v *Order) { v.DeliveryRegion = "" }}
	for index, mutate := range tests {
		value := validOrder()
		mutate(&value)
		if err := value.Validate(); !errors.Is(err, ErrInvalid) {
			t.Fatalf("case %d error=%v", index, err)
		}
	}
}

func TestOrderValidationRejectsLines(t *testing.T) {
	t.Parallel()
	value := validOrder()
	value.Lines = nil
	if err := value.Validate(); !errors.Is(err, ErrInvalid) {
		t.Fatalf("empty lines error=%v", err)
	}
	mutations := []func(*OrderLine){func(v *OrderLine) { v.Quantity = 0 }, func(v *OrderLine) { v.Quantity = -1 }, func(v *OrderLine) { v.UnitPriceCents = 0 }, func(v *OrderLine) { v.Species = "" }}
	for index, mutate := range mutations {
		value = validOrder()
		mutate(&value.Lines[0])
		if err := value.Validate(); !errors.Is(err, ErrInvalid) {
			t.Fatalf("line case %d error=%v", index, err)
		}
	}
}

func TestOrderValidationRejectsDueDate(t *testing.T) {
	t.Parallel()
	value := validOrder()
	value.DueAt = value.RequestedAt
	if err := value.Validate(); !errors.Is(err, ErrInvalid) {
		t.Fatalf("same due date error=%v", err)
	}
	value.DueAt = value.RequestedAt.Add(-time.Second)
	if err := value.Validate(); !errors.Is(err, ErrInvalid) {
		t.Fatalf("past due date error=%v", err)
	}
}

func TestSessionActiveAtHonorsRevocationAndExpiry(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 8, 25, 0, 0, 0, 0, time.UTC)
	session := Session{ExpiresAt: now.Add(time.Hour)}
	if !session.ActiveAt(now) {
		t.Fatal("fresh session inactive")
	}
	if session.ActiveAt(now.Add(time.Hour)) {
		t.Fatal("session active at exact expiry")
	}
	revoked := now
	session.RevokedAt = &revoked
	if session.ActiveAt(now) {
		t.Fatal("revoked session active")
	}
}
