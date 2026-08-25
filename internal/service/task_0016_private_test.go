package service

import (
	"errors"
	"testing"
	"time"

	"github.com/VanceMichael/go-label-yanji-mushroomchain-g12-v1/internal/domain"
)

func TestAllocateOrderRollsBackPartialInventory(t *testing.T) {
	f := newServiceFixture(t)
	batch := f.data.Batch(domain.BatchReleased, 5)
	batch.ID = "batch-oyster-only"
	batch.Code = "BATCH-OYSTER-ONLY"
	batch.Species = "oyster"
	if err := f.data.Store.CreateBatch(t.Context(), batch); err != nil {
		t.Fatal(err)
	}
	order, err := f.orders.Create(f.ctx(domain.RoleDispatcher), CreateOrderInput{
		BuyerName: "Two-line buyer", DeliveryRegion: "Yinchuan", IdempotencyKey: "partial-allocation",
		DueAt: f.data.Now.Add(time.Hour), Lines: []domain.OrderLine{
			{Species: "oyster", Quantity: 5, UnitPriceCents: 400},
			{Species: "shiitake", Quantity: 4, UnitPriceCents: 500},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.orders.Allocate(f.ctx(domain.RoleDispatcher), AllocateInput{OrderID: order.ID, ExpectedVersion: order.Version, RequestID: "allocate-partial"}); !errors.Is(err, domain.ErrCapacity) {
		t.Fatalf("allocate error=%v", err)
	}
	stored, err := f.data.Store.GetBatch(t.Context(), f.data.TenantID, batch.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.QuantityAvailable != 5 || stored.Status != domain.BatchReleased || stored.Version != 1 {
		t.Fatalf("inventory was consumed after failed multi-line allocation: %+v", stored)
	}
	allocations, err := f.data.Store.ListAllocations(t.Context(), f.data.TenantID, order.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(allocations) != 0 {
		t.Fatalf("partial allocations remained after capacity failure: %+v", allocations)
	}
	storedOrder, err := f.data.Store.GetOrder(t.Context(), f.data.TenantID, order.ID)
	if err != nil {
		t.Fatal(err)
	}
	if storedOrder.Status != domain.OrderConfirmed || storedOrder.Version != order.Version {
		t.Fatalf("order state changed after failed allocation: %+v", storedOrder)
	}
}
