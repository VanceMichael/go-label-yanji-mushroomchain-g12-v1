package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/VanceMichael/go-label-yanji-mushroomchain-g12-v1/internal/domain"
	"github.com/VanceMichael/go-label-yanji-mushroomchain-g12-v1/internal/pagination"
	"github.com/VanceMichael/go-label-yanji-mushroomchain-g12-v1/internal/repository"
)

type deliveryFailingRepo struct {
	repository.Tx
	base            repository.Transactor
	settlementCalls int
	failOnCall      int
}

func (r *deliveryFailingRepo) WithinTx(ctx context.Context, fn func(repository.Tx) error) error {
	return r.base.WithinTx(ctx, func(tx repository.Tx) error {
		return fn(&deliveryFailingTx{Tx: tx, owner: r})
	})
}

func (r *deliveryFailingRepo) CreateSettlement(ctx context.Context, item domain.Settlement) error {
	return r.createSettlement(ctx, r.Tx, item)
}

type deliveryFailingTx struct {
	repository.Tx
	owner *deliveryFailingRepo
}

func (r *deliveryFailingTx) CreateSettlement(ctx context.Context, item domain.Settlement) error {
	return r.owner.createSettlement(ctx, r.Tx, item)
}

func (r *deliveryFailingRepo) createSettlement(ctx context.Context, target repository.Tx, item domain.Settlement) error {
	r.settlementCalls++
	if r.failOnCall > 0 && r.settlementCalls == r.failOnCall {
		return errors.New("injected second-farm settlement failure")
	}
	return target.CreateSettlement(ctx, item)
}

func TestDeliveryFailureRollsBackOrderAllFarmSettlementsAndEvents(t *testing.T) {
	f := newServiceFixture(t)
	ctx := f.ctx(domain.RoleDispatcher)
	secondFarm := domain.Farm{ID: "farm-two", TenantID: f.data.TenantID, Name: "Shatang Farm", Village: "Shatang", OwnerUserID: f.data.Users[domain.RoleFarmer].ID, SettlementName: "Farmer Two", CreatedAt: f.data.Now}
	if err := f.data.Store.CreateFarm(context.Background(), secondFarm); err != nil {
		t.Fatal(err)
	}
	firstBatch := f.data.Batch(domain.BatchReleased, 8)
	firstBatch.ID, firstBatch.Code, firstBatch.ExpiresAt = "batch-farm-one", "FARM-ONE", f.data.Now.Add(24*time.Hour)
	secondBatch := f.data.Batch(domain.BatchReleased, 8)
	secondBatch.ID, secondBatch.FarmID, secondBatch.Code, secondBatch.ExpiresAt = "batch-farm-two", secondFarm.ID, "FARM-TWO", f.data.Now.Add(48*time.Hour)
	for _, batch := range []domain.SubstrateBatch{firstBatch, secondBatch} {
		if err := f.data.Store.CreateBatch(context.Background(), batch); err != nil {
			t.Fatal(err)
		}
	}
	order, err := f.orders.Create(ctx, CreateOrderInput{BuyerName: "Two Farm Buyer", DeliveryRegion: "Yinchuan", IdempotencyKey: "two-farm-delivery", DueAt: f.data.Now.Add(24 * time.Hour), Lines: []domain.OrderLine{{Species: "oyster", Quantity: 16, UnitPriceCents: 400}}})
	if err != nil {
		t.Fatal(err)
	}
	allocated, err := f.orders.Allocate(ctx, AllocateInput{OrderID: order.ID, ExpectedVersion: order.Version, RequestID: "allocate-two-farm"})
	if err != nil {
		t.Fatal(err)
	}
	wrapped := &deliveryFailingRepo{Tx: f.data.Store, base: f.data.Store, failOnCall: 2}
	delivery := NewOrders(wrapped, wrapped, f.clock)
	if _, err = delivery.MarkDelivered(ctx, DeliveryInput{OrderID: order.ID, ExpectedVersion: allocated.Version, RequestID: "deliver-failure"}); err == nil {
		t.Fatal("expected second-farm settlement failure")
	}
	stored, err := f.data.Store.GetOrder(context.Background(), f.data.TenantID, order.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != domain.OrderAllocated {
		t.Fatalf("failed delivery changed order status to %s", stored.Status)
	}
	settlements, err := f.data.Store.ListSettlementsByOrder(context.Background(), f.data.TenantID, order.ID)
	if err != nil || len(settlements) != 0 {
		t.Fatalf("partial settlements=%+v err=%v", settlements, err)
	}
	claimed, claimErr := f.data.Store.ClaimOutbox(context.Background(), "private-check", f.data.Now, f.data.Now.Add(time.Minute))
	if !errors.Is(claimErr, domain.ErrNotFound) {
		t.Fatalf("partial outbox event=%+v err=%v", claimed, claimErr)
	}
	auditEvents, _, err := f.data.Store.ListAudit(context.Background(), f.data.TenantID, "order", order.ID, repositoryPage())
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range auditEvents {
		if event.Action == "order.deliver" {
			t.Fatalf("failed delivery left audit event=%+v", event)
		}
	}

	wrapped.failOnCall = 0
	wrapped.settlementCalls = 0
	delivered, err := delivery.MarkDelivered(ctx, DeliveryInput{OrderID: order.ID, ExpectedVersion: allocated.Version, RequestID: "deliver-retry"})
	if err != nil {
		t.Fatalf("retry delivery: %v", err)
	}
	if delivered.Status != domain.OrderDelivered {
		t.Fatalf("retry status=%s", delivered.Status)
	}
	settlements, err = f.data.Store.ListSettlementsByOrder(context.Background(), f.data.TenantID, order.ID)
	if err != nil || len(settlements) != 2 {
		t.Fatalf("retry settlements=%+v err=%v", settlements, err)
	}
	for _, settlement := range settlements {
		if settlement.AmountCents != 8*350 {
			t.Fatalf("settlement amount=%d", settlement.AmountCents)
		}
	}
	first, firstErr := f.data.Store.ClaimOutbox(context.Background(), "private-check-1", f.data.Now, f.data.Now.Add(time.Minute))
	second, secondErr := f.data.Store.ClaimOutbox(context.Background(), "private-check-2", f.data.Now, f.data.Now.Add(time.Minute))
	if firstErr != nil || secondErr != nil || first.ID == "" || second.ID == "" {
		t.Fatalf("retry outbox first=%+v/%v second=%+v/%v", first, firstErr, second, secondErr)
	}
	auditEvents, _, err = f.data.Store.ListAudit(context.Background(), f.data.TenantID, "order", order.ID, repositoryPage())
	if err != nil {
		t.Fatal(err)
	}
	deliveryAudits := 0
	for _, event := range auditEvents {
		if event.Action == "order.deliver" {
			deliveryAudits++
		}
	}
	if deliveryAudits != 1 {
		t.Fatalf("delivery audits=%d events=%+v", deliveryAudits, auditEvents)
	}
}

func repositoryPage() pagination.Page { return pagination.Page{Number: 1, Size: 50} }
