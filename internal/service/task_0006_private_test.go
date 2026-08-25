package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/VanceMichael/go-label-yanji-mushroomchain-g12-v1/internal/domain"
)

func TestAllocateOrderRollsBackAllLinesWhenLaterLineLacksCapacity(t *testing.T) {
	f := newServiceFixture(t)
	available := f.data.Batch(domain.BatchReleased, 5)
	available.ID = "batch-oyster-only"
	available.Code = "XB-OYSTER-ONLY"
	if err := f.data.Store.CreateBatch(context.Background(), available); err != nil {
		t.Fatal(err)
	}
	order, err := f.orders.Create(f.ctx(domain.RoleDispatcher), CreateOrderInput{
		BuyerName:      "Two species buyer",
		DeliveryRegion: "Yinchuan",
		IdempotencyKey: "two-species-capacity",
		DueAt:          f.data.Now.Add(time.Hour),
		Lines: []domain.OrderLine{
			{Species: "oyster", Quantity: 5, UnitPriceCents: 400},
			{Species: "shiitake", Quantity: 3, UnitPriceCents: 500},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.orders.Allocate(f.ctx(domain.RoleDispatcher), AllocateInput{OrderID: order.ID, ExpectedVersion: order.Version, RequestID: "allocate-two-species"}); !errors.Is(err, domain.ErrCapacity) {
		t.Fatalf("allocate error=%v", err)
	}
	storedBatch, err := f.data.Store.GetBatch(context.Background(), f.data.TenantID, available.ID)
	if err != nil {
		t.Fatal(err)
	}
	if storedBatch.QuantityAvailable != 5 || storedBatch.Version != 1 {
		t.Fatalf("partial inventory persisted: %+v", storedBatch)
	}
	allocations, err := f.data.Store.ListAllocations(context.Background(), f.data.TenantID, order.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(allocations) != 0 {
		t.Fatalf("partial allocations persisted: %+v", allocations)
	}
	storedOrder, err := f.data.Store.GetOrder(context.Background(), f.data.TenantID, order.ID)
	if err != nil {
		t.Fatal(err)
	}
	if storedOrder.Status != domain.OrderConfirmed || storedOrder.Version != order.Version {
		t.Fatalf("order state changed after capacity failure: %+v", storedOrder)
	}
}
