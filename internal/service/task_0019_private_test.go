package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/VanceMichael/go-label-yanji-mushroomchain-g12-v1/internal/clock"
	"github.com/VanceMichael/go-label-yanji-mushroomchain-g12-v1/internal/domain"
	"github.com/VanceMichael/go-label-yanji-mushroomchain-g12-v1/internal/repository"
	"github.com/VanceMichael/go-label-yanji-mushroomchain-g12-v1/internal/testkit"
)

type task0019BatchReadBarrier struct {
	repository.Tx
	ready       chan<- struct{}
	release     <-chan struct{}
	operationMu *sync.Mutex
}

func (r task0019BatchReadBarrier) ListBatches(ctx context.Context, filter repository.BatchFilter) ([]domain.SubstrateBatch, int, error) {
	items, total, err := r.Tx.ListBatches(ctx, filter)
	if err != nil {
		return nil, 0, err
	}
	select {
	case r.ready <- struct{}{}:
	case <-ctx.Done():
		return nil, 0, ctx.Err()
	}
	select {
	case <-r.release:
		return items, total, nil
	case <-ctx.Done():
		return nil, 0, ctx.Err()
	}
}

func (r task0019BatchReadBarrier) CreateAllocations(ctx context.Context, items []domain.InventoryAllocation) error {
	r.operationMu.Lock()
	defer r.operationMu.Unlock()
	return r.Tx.CreateAllocations(ctx, items)
}

type task0019SerializedTransactor struct {
	repository.Transactor
	operationMu *sync.Mutex
}

func (t *task0019SerializedTransactor) WithinTx(ctx context.Context, fn func(repository.Tx) error) error {
	t.operationMu.Lock()
	defer t.operationMu.Unlock()
	return t.Transactor.WithinTx(ctx, fn)
}

func TestConcurrentOrderAllocationHasSingleInventoryWinner(t *testing.T) {
	data := testkit.New(t)
	fixed := clock.NewFixed(data.Now)
	dispatcher := data.Users[domain.RoleDispatcher]
	dispatcherCtx := WithIdentity(context.Background(), Identity{
		UserID: dispatcher.ID, TenantID: dispatcher.TenantID, FarmID: dispatcher.FarmID,
		SessionID: "task-0019-session", Role: domain.RoleDispatcher,
	})

	batch := data.Batch(domain.BatchReleased, 8)
	batch.ID = "task-0019-shared-batch"
	batch.Code = "TASK-0019-SHARED"
	if err := data.Store.CreateBatch(context.Background(), batch); err != nil {
		t.Fatalf("create shared batch: %v", err)
	}

	setupOrders := NewOrders(data.Store, data.Store, fixed)
	create := func(key, buyer string) domain.Order {
		order, err := setupOrders.Create(dispatcherCtx, CreateOrderInput{
			BuyerName: buyer, DeliveryRegion: "Yinchuan", IdempotencyKey: key,
			RequestID: key, DueAt: data.Now.Add(24 * time.Hour),
			Lines: []domain.OrderLine{{Species: batch.Species, Quantity: 8, UnitPriceCents: 400}},
		})
		if err != nil {
			t.Fatalf("create %s: %v", key, err)
		}
		return order
	}
	orders := []domain.Order{create("task-0019-order-a", "Buyer A"), create("task-0019-order-b", "Buyer B")}

	ready := make(chan struct{}, 2)
	release := make(chan struct{})
	operationMu := &sync.Mutex{}
	repo := task0019BatchReadBarrier{Tx: data.Store, ready: ready, release: release, operationMu: operationMu}
	tx := &task0019SerializedTransactor{Transactor: data.Store, operationMu: operationMu}
	allocationService := NewOrders(tx, repo, fixed)
	type result struct {
		orderID string
		order   domain.Order
		err     error
	}
	results := make(chan result, len(orders))
	for _, order := range orders {
		order := order
		go func() {
			allocated, err := allocationService.Allocate(dispatcherCtx, AllocateInput{
				OrderID: order.ID, ExpectedVersion: order.Version, RequestID: "allocate-" + order.ID,
			})
			results <- result{orderID: order.ID, order: allocated, err: err}
		}()
	}
	<-ready
	<-ready
	close(release)

	var winnerID, loserID string
	for range orders {
		outcome := <-results
		if outcome.err == nil {
			if winnerID != "" {
				t.Fatalf("both allocations succeeded: first=%s second=%s", winnerID, outcome.orderID)
			}
			winnerID = outcome.orderID
			if outcome.order.Status != domain.OrderAllocated {
				t.Fatalf("winner status=%s", outcome.order.Status)
			}
			continue
		}
		if !errors.Is(outcome.err, domain.ErrVersionConflict) && !errors.Is(outcome.err, domain.ErrCapacity) {
			t.Fatalf("losing allocation error=%v", outcome.err)
		}
		loserID = outcome.orderID
	}
	if winnerID == "" || loserID == "" {
		t.Fatalf("winner=%q loser=%q", winnerID, loserID)
	}

	winner, err := data.Store.GetOrder(context.Background(), data.TenantID, winnerID)
	if err != nil || winner.Status != domain.OrderAllocated {
		t.Fatalf("winner=%+v err=%v", winner, err)
	}
	winnerAllocations, err := data.Store.ListAllocations(context.Background(), data.TenantID, winnerID)
	if err != nil || len(winnerAllocations) != 1 || winnerAllocations[0].BatchID != batch.ID || winnerAllocations[0].Quantity != 8 {
		t.Fatalf("winner allocations=%+v err=%v", winnerAllocations, err)
	}
	if _, err = data.Store.GetShipmentByOrder(context.Background(), data.TenantID, winnerID); err != nil {
		t.Fatalf("winner shipment: %v", err)
	}

	loser, err := data.Store.GetOrder(context.Background(), data.TenantID, loserID)
	if err != nil || loser.Status != domain.OrderConfirmed {
		t.Fatalf("loser=%+v err=%v", loser, err)
	}
	loserAllocations, err := data.Store.ListAllocations(context.Background(), data.TenantID, loserID)
	if err != nil || len(loserAllocations) != 0 {
		t.Fatalf("loser allocations=%+v err=%v", loserAllocations, err)
	}
	if _, err = data.Store.GetShipmentByOrder(context.Background(), data.TenantID, loserID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("loser shipment error=%v", err)
	}

	storedBatch, err := data.Store.GetBatch(context.Background(), data.TenantID, batch.ID)
	if err != nil || storedBatch.QuantityAvailable != 0 || storedBatch.Version != batch.Version+1 {
		t.Fatalf("shared batch=%+v err=%v", storedBatch, err)
	}
}
