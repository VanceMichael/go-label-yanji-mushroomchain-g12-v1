package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/VanceMichael/go-label-yanji-mushroomchain-g12-v1/internal/domain"
)

func TestIdempotencyRejectsChangedOrderPayload(t *testing.T) {
	f := newServiceFixture(t)
	ctx := f.ctx(domain.RoleDispatcher)
	firstInput := CreateOrderInput{
		BuyerName:      "Qingshan Cooperative",
		DeliveryRegion: "Yinchuan North",
		IdempotencyKey: "order-retry-0007",
		RequestID:      "request-first",
		DueAt:          f.data.Now.Add(24 * time.Hour),
		Lines:          []domain.OrderLine{{Species: "oyster", Quantity: 12, UnitPriceCents: 400}},
	}
	first, err := f.orders.Create(ctx, firstInput)
	if err != nil {
		t.Fatalf("create first order: %v", err)
	}
	replay, err := f.orders.Create(ctx, firstInput)
	if err != nil {
		t.Fatalf("replay same payload: %v", err)
	}
	if replay.ID != first.ID || replay.BuyerName != first.BuyerName || replay.TotalCents != first.TotalCents {
		t.Fatalf("same payload did not replay original order: first=%+v replay=%+v", first, replay)
	}

	changed := firstInput
	changed.RequestID = "request-changed"
	changed.BuyerName = "Longhe Distributor"
	changed.DeliveryRegion = "Helan East"
	changed.Lines = []domain.OrderLine{{Species: "shiitake", Quantity: 4, UnitPriceCents: 900}}
	changedOrder, err := f.orders.Create(ctx, changed)
	if err == nil || !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("changed payload must return conflict, got order=%+v err=%v", changedOrder, err)
	}
	stored, err := f.data.Store.GetOrder(context.Background(), f.data.TenantID, first.ID)
	if err != nil {
		t.Fatalf("reload original order: %v", err)
	}
	if stored.BuyerName != first.BuyerName || stored.DeliveryRegion != first.DeliveryRegion || len(stored.Lines) != 1 || stored.Lines[0].Species != "oyster" {
		t.Fatalf("changed retry altered original order: %+v", stored)
	}
}
