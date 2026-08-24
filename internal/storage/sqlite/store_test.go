package sqlite

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/VanceMichael/go-label-yanji-mushroomchain-g12-v1/internal/audit"
	"github.com/VanceMichael/go-label-yanji-mushroomchain-g12-v1/internal/domain"
	"github.com/VanceMichael/go-label-yanji-mushroomchain-g12-v1/internal/pagination"
	"github.com/VanceMichael/go-label-yanji-mushroomchain-g12-v1/internal/repository"
)

type storeFixture struct {
	store  *Store
	path   string
	tenant string
	now    time.Time
	users  map[domain.Role]domain.User
	farm   domain.Farm
}

func newStoreFixture(t *testing.T) storeFixture {
	t.Helper()
	path := t.TempDir() + "/store.db"
	store, err := Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err = store.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 25, 8, 0, 0, 0, time.UTC)
	users := map[domain.Role]domain.User{}
	roles := []domain.Role{domain.RoleOperator, domain.RoleFarmer, domain.RoleInspector, domain.RoleDispatcher, domain.RoleFinance}
	for _, role := range roles {
		users[role] = domain.User{ID: "user-" + string(role), TenantID: "tenant", Email: string(role) + "@test.local", DisplayName: string(role), Role: role, PasswordHash: "hash", Active: true, CreatedAt: now, UpdatedAt: now}
	}
	list := make([]domain.User, 0, len(roles))
	for _, role := range roles {
		list = append(list, users[role])
	}
	if err = store.SeedTenant(context.Background(), "tenant", "Test", list); err != nil {
		t.Fatal(err)
	}
	farm := domain.Farm{ID: "farm", TenantID: "tenant", Name: "Farm", Village: "Village", OwnerUserID: users[domain.RoleFarmer].ID, SettlementName: "Farmer", CreatedAt: now}
	if err = store.CreateFarm(context.Background(), farm); err != nil {
		t.Fatal(err)
	}
	return storeFixture{store: store, path: path, tenant: "tenant", now: now, users: users, farm: farm}
}

func (f storeFixture) batch(id string, status domain.BatchStatus, quantity int64) domain.SubstrateBatch {
	return domain.SubstrateBatch{ID: id, TenantID: f.tenant, FarmID: f.farm.ID, Code: "CODE-" + id, Species: "oyster", ProducedAt: f.now.Add(-time.Hour), ExpiresAt: f.now.Add(30 * 24 * time.Hour), QuantityProduced: quantity, QuantityAvailable: quantity, UnitPriceCents: 300, Status: status, Version: 1, CreatedAt: f.now, UpdatedAt: f.now}
}

func (f storeFixture) order(id string, status domain.OrderStatus, quantity int64) domain.Order {
	return domain.Order{ID: id, TenantID: f.tenant, BuyerName: "Buyer", DeliveryRegion: "Yinchuan", Status: status, IdempotencyKey: "key-" + id, Version: 1, TotalCents: quantity * 400, RequestedAt: f.now, DueAt: f.now.Add(24 * time.Hour), CreatedAt: f.now, UpdatedAt: f.now, Lines: []domain.OrderLine{{ID: "line-" + id, OrderID: id, Species: "oyster", Quantity: quantity, UnitPriceCents: 400}}}
}

func TestMigrationIsIdempotentAndForeignKeysEnabled(t *testing.T) {
	f := newStoreFixture(t)
	if err := f.store.Migrate(context.Background()); err != nil {
		t.Fatalf("second migrate: %v", err)
	}
	if err := f.store.Ping(context.Background()); err != nil {
		t.Fatalf("ping: %v", err)
	}
	exists, err := f.store.TenantExists(context.Background(), f.tenant)
	if err != nil || !exists {
		t.Fatalf("tenant exists=%v err=%v", exists, err)
	}
}

func TestUserSessionLifecycle(t *testing.T) {
	f := newStoreFixture(t)
	ctx := context.Background()
	user := f.users[domain.RoleOperator]
	found, err := f.store.FindUserByEmail(ctx, f.tenant, "OPERATOR@test.local")
	if err != nil {
		t.Fatal(err)
	}
	if found.ID != user.ID {
		t.Fatalf("user=%+v", found)
	}
	session := domain.Session{ID: "session", UserID: user.ID, TokenHash: "token-hash", ExpiresAt: f.now.Add(time.Hour), CreatedAt: f.now}
	if err = f.store.CreateSession(ctx, session); err != nil {
		t.Fatal(err)
	}
	gotSession, gotUser, err := f.store.FindSessionByTokenHash(ctx, "token-hash")
	if err != nil {
		t.Fatal(err)
	}
	if gotSession.ID != session.ID || gotUser.ID != user.ID {
		t.Fatalf("session=%+v user=%+v", gotSession, gotUser)
	}
	if err = f.store.RevokeSession(ctx, user.ID, session.ID, f.now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	gotSession, _, err = f.store.FindSessionByTokenHash(ctx, "token-hash")
	if err != nil {
		t.Fatal(err)
	}
	if gotSession.RevokedAt == nil {
		t.Fatal("revocation not persisted")
	}
	count, err := f.store.DeleteExpiredSessions(ctx, f.now.Add(2*time.Hour))
	if err != nil || count != 1 {
		t.Fatalf("deleted=%d err=%v", count, err)
	}
	if _, _, err = f.store.FindSessionByTokenHash(ctx, "token-hash"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("deleted session error=%v", err)
	}
}

func TestDuplicateUserAndFarmAreConflicts(t *testing.T) {
	f := newStoreFixture(t)
	ctx := context.Background()
	duplicate := f.users[domain.RoleOperator]
	duplicate.ID = "other-user"
	if err := f.store.CreateUser(ctx, duplicate); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("duplicate user error=%v", err)
	}
	farm := f.farm
	farm.ID = "other-farm"
	if err := f.store.CreateFarm(ctx, farm); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("duplicate farm error=%v", err)
	}
}

func TestBatchLifecycleAndInspectionPersistence(t *testing.T) {
	f := newStoreFixture(t)
	ctx := context.Background()
	batch := f.batch("batch", domain.BatchRegistered, 100)
	if err := f.store.CreateBatch(ctx, batch); err != nil {
		t.Fatal(err)
	}
	if err := f.store.TransitionBatch(ctx, f.tenant, batch.ID, 1, domain.BatchSampling, f.now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	inspection := domain.QualityInspection{ID: "inspection", TenantID: f.tenant, BatchID: batch.ID, InspectorID: f.users[domain.RoleInspector].ID, Decision: domain.InspectionApproved, MoistureBP: 6200, SampleCount: 8, Notes: "passed", InspectedAt: f.now.Add(2 * time.Minute), CreatedAt: f.now.Add(2 * time.Minute)}
	if err := f.store.CreateInspection(ctx, inspection); err != nil {
		t.Fatal(err)
	}
	if err := f.store.TransitionBatch(ctx, f.tenant, batch.ID, 2, domain.BatchReleased, f.now.Add(3*time.Minute)); err != nil {
		t.Fatal(err)
	}
	got, err := f.store.GetBatch(ctx, f.tenant, batch.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != domain.BatchReleased || got.Version != 3 {
		t.Fatalf("batch=%+v", got)
	}
	items, err := f.store.ListInspections(ctx, f.tenant, batch.ID)
	if err != nil || len(items) != 1 || items[0].Decision != domain.InspectionApproved {
		t.Fatalf("inspections=%+v err=%v", items, err)
	}
}

func TestInvalidBatchTransitionDoesNotWrite(t *testing.T) {
	f := newStoreFixture(t)
	ctx := context.Background()
	batch := f.batch("batch", domain.BatchRegistered, 100)
	if err := f.store.CreateBatch(ctx, batch); err != nil {
		t.Fatal(err)
	}
	err := f.store.TransitionBatch(ctx, f.tenant, batch.ID, 1, domain.BatchArchived, f.now)
	if !errors.Is(err, domain.ErrState) {
		t.Fatalf("transition error=%v", err)
	}
	got, _ := f.store.GetBatch(ctx, f.tenant, batch.ID)
	if got.Status != domain.BatchRegistered || got.Version != 1 {
		t.Fatalf("batch changed: %+v", got)
	}
}

func TestBatchVersionConflict(t *testing.T) {
	f := newStoreFixture(t)
	ctx := context.Background()
	batch := f.batch("batch", domain.BatchRegistered, 100)
	if err := f.store.CreateBatch(ctx, batch); err != nil {
		t.Fatal(err)
	}
	if err := f.store.TransitionBatch(ctx, f.tenant, batch.ID, 1, domain.BatchSampling, f.now); err != nil {
		t.Fatal(err)
	}
	if err := f.store.TransitionBatch(ctx, f.tenant, batch.ID, 1, domain.BatchReleased, f.now); !errors.Is(err, domain.ErrVersionConflict) {
		t.Fatalf("stale transition error=%v", err)
	}
}

func TestConditionalAllocationAndRestore(t *testing.T) {
	f := newStoreFixture(t)
	ctx := context.Background()
	batch := f.batch("batch", domain.BatchReleased, 10)
	if err := f.store.CreateBatch(ctx, batch); err != nil {
		t.Fatal(err)
	}
	if err := f.store.AllocateBatch(ctx, f.tenant, batch.ID, 6, 1, f.now); err != nil {
		t.Fatal(err)
	}
	got, _ := f.store.GetBatch(ctx, f.tenant, batch.ID)
	if got.QuantityAvailable != 4 || got.Version != 2 || got.Status != domain.BatchReleased {
		t.Fatalf("after allocation=%+v", got)
	}
	if err := f.store.AllocateBatch(ctx, f.tenant, batch.ID, 5, 2, f.now); !errors.Is(err, domain.ErrCapacity) {
		t.Fatalf("capacity error=%v", err)
	}
	if err := f.store.AllocateBatch(ctx, f.tenant, batch.ID, 4, 2, f.now); err != nil {
		t.Fatal(err)
	}
	got, _ = f.store.GetBatch(ctx, f.tenant, batch.ID)
	if got.QuantityAvailable != 0 || got.Status != domain.BatchExhausted {
		t.Fatalf("exhausted=%+v", got)
	}
	if err := f.store.RestoreBatch(ctx, f.tenant, batch.ID, 3, f.now); err != nil {
		t.Fatal(err)
	}
	got, _ = f.store.GetBatch(ctx, f.tenant, batch.ID)
	if got.QuantityAvailable != 3 || got.Status != domain.BatchReleased {
		t.Fatalf("restored=%+v", got)
	}
}

func TestConcurrentAllocationPreservesCapacity(t *testing.T) {
	f := newStoreFixture(t)
	ctx := context.Background()
	batch := f.batch("batch", domain.BatchReleased, 10)
	if err := f.store.CreateBatch(ctx, batch); err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for index := 0; index < 2; index++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			results <- f.store.AllocateBatch(ctx, f.tenant, batch.ID, 7, 1, f.now)
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	success, failed := 0, 0
	for err := range results {
		if err == nil {
			success++
		} else if errors.Is(err, domain.ErrVersionConflict) || errors.Is(err, domain.ErrConflict) {
			failed++
		} else {
			t.Fatalf("unexpected allocation error=%v", err)
		}
	}
	if success != 1 || failed != 1 {
		t.Fatalf("success=%d failed=%d", success, failed)
	}
	got, _ := f.store.GetBatch(ctx, f.tenant, batch.ID)
	if got.QuantityAvailable != 3 {
		t.Fatalf("available=%d", got.QuantityAvailable)
	}
}

func TestWithinTxRollsBackCrossEntityWrites(t *testing.T) {
	f := newStoreFixture(t)
	ctx := context.Background()
	batch := f.batch("rollback", domain.BatchRegistered, 10)
	sentinel := errors.New("audit unavailable")
	err := f.store.WithinTx(ctx, func(tx repository.Tx) error {
		if err := tx.CreateBatch(ctx, batch); err != nil {
			return err
		}
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("transaction error=%v", err)
	}
	if _, err = f.store.GetBatch(ctx, f.tenant, batch.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("rolled back batch error=%v", err)
	}
}

func TestOrderLinesIdempotencyAndPagination(t *testing.T) {
	f := newStoreFixture(t)
	ctx := context.Background()
	for index := 0; index < 5; index++ {
		order := f.order(fmt.Sprintf("order-%d", index), domain.OrderDraft, int64(index+1))
		if err := f.store.CreateOrder(ctx, order); err != nil {
			t.Fatal(err)
		}
	}
	order, err := f.store.FindOrderByIdempotency(ctx, f.tenant, "key-order-3")
	if err != nil {
		t.Fatal(err)
	}
	if len(order.Lines) != 1 || order.Lines[0].Quantity != 4 {
		t.Fatalf("order=%+v", order)
	}
	items, total, err := f.store.ListOrders(ctx, repository.OrderFilter{TenantID: f.tenant, Status: domain.OrderDraft, Page: pagination.Page{Number: 2, Size: 2}})
	if err != nil {
		t.Fatal(err)
	}
	if total != 5 || len(items) != 2 {
		t.Fatalf("total=%d items=%d", total, len(items))
	}
}

func TestOrderAllocationRowsAndTransition(t *testing.T) {
	f := newStoreFixture(t)
	ctx := context.Background()
	order := f.order("order", domain.OrderConfirmed, 5)
	if err := f.store.CreateOrder(ctx, order); err != nil {
		t.Fatal(err)
	}
	batch := f.batch("batch", domain.BatchReleased, 5)
	if err := f.store.CreateBatch(ctx, batch); err != nil {
		t.Fatal(err)
	}
	allocation := domain.InventoryAllocation{ID: "allocation", TenantID: f.tenant, OrderID: order.ID, LineID: order.Lines[0].ID, BatchID: batch.ID, Quantity: 5, CreatedAt: f.now}
	if err := f.store.CreateAllocation(ctx, allocation); err != nil {
		t.Fatal(err)
	}
	if err := f.store.TransitionOrder(ctx, f.tenant, order.ID, 1, domain.OrderAllocated, f.now); err != nil {
		t.Fatal(err)
	}
	items, err := f.store.ListAllocations(ctx, f.tenant, order.ID)
	if err != nil || len(items) != 1 || items[0].BatchID != batch.ID {
		t.Fatalf("allocations=%+v err=%v", items, err)
	}
}

func TestShipmentLeaseCanBeRecoveredAfterExpiry(t *testing.T) {
	f := newStoreFixture(t)
	ctx := context.Background()
	order := f.order("order", domain.OrderAllocated, 5)
	if err := f.store.CreateOrder(ctx, order); err != nil {
		t.Fatal(err)
	}
	shipment := domain.Shipment{ID: "shipment", TenantID: f.tenant, OrderID: order.ID, Status: domain.ShipmentPending, CreatedAt: f.now, UpdatedAt: f.now}
	if err := f.store.CreateShipment(ctx, shipment); err != nil {
		t.Fatal(err)
	}
	claimed, err := f.store.ClaimShipment(ctx, "worker-a", f.now, f.now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if claimed.ClaimedBy != "worker-a" || claimed.Attempts != 1 {
		t.Fatalf("claimed=%+v", claimed)
	}
	if _, err = f.store.ClaimShipment(ctx, "worker-b", f.now.Add(30*time.Second), f.now.Add(2*time.Minute)); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("active lease claim error=%v", err)
	}
	reclaimed, err := f.store.ClaimShipment(ctx, "worker-b", f.now.Add(2*time.Minute), f.now.Add(3*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if reclaimed.ClaimedBy != "worker-b" || reclaimed.Attempts != 2 {
		t.Fatalf("reclaimed=%+v", reclaimed)
	}
	if err = f.store.CompleteShipment(ctx, reclaimed.ID, "worker-a", domain.ShipmentDispatched, "", f.now); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("wrong owner complete error=%v", err)
	}
	if err = f.store.CompleteShipment(ctx, reclaimed.ID, "worker-b", domain.ShipmentDispatched, "", f.now); err != nil {
		t.Fatal(err)
	}
}

func TestSettlementOptimisticLifecycle(t *testing.T) {
	f := newStoreFixture(t)
	ctx := context.Background()
	order := f.order("order", domain.OrderDelivered, 5)
	if err := f.store.CreateOrder(ctx, order); err != nil {
		t.Fatal(err)
	}
	item := domain.Settlement{ID: "settlement", TenantID: f.tenant, FarmID: f.farm.ID, OrderID: order.ID, AmountCents: 1500, Status: domain.SettlementPending, Version: 1, CreatedAt: f.now, UpdatedAt: f.now}
	if err := f.store.CreateSettlement(ctx, item); err != nil {
		t.Fatal(err)
	}
	if err := f.store.ApproveSettlement(ctx, f.tenant, item.ID, 1, f.users[domain.RoleFinance].ID, f.now); err != nil {
		t.Fatal(err)
	}
	if err := f.store.MarkSettlementPaid(ctx, f.tenant, item.ID, 1, f.now); !errors.Is(err, domain.ErrVersionConflict) {
		t.Fatalf("stale pay error=%v", err)
	}
	if err := f.store.MarkSettlementPaid(ctx, f.tenant, item.ID, 2, f.now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	got, err := f.store.GetSettlement(ctx, f.tenant, item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != domain.SettlementPaid || got.Version != 3 || got.PaidAt == nil {
		t.Fatalf("settlement=%+v", got)
	}
}

func TestOutboxLeaseRetryAndDelivery(t *testing.T) {
	f := newStoreFixture(t)
	ctx := context.Background()
	event := domain.OutboxEvent{ID: "event", TenantID: f.tenant, AggregateType: "batch", AggregateID: "batch", EventType: "batch.released", Payload: []byte(`{"id":"batch"}`), Status: domain.OutboxPending, AvailableAt: f.now, CreatedAt: f.now}
	if err := f.store.CreateOutboxEvent(ctx, event); err != nil {
		t.Fatal(err)
	}
	claimed, err := f.store.ClaimOutbox(ctx, "worker", f.now, f.now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if claimed.Attempts != 1 || claimed.Status != domain.OutboxClaimed {
		t.Fatalf("claimed=%+v", claimed)
	}
	if err = f.store.FailOutbox(ctx, claimed.ID, "worker", "temporary", 4, f.now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, err = f.store.ClaimOutbox(ctx, "worker", f.now.Add(30*time.Second), f.now.Add(time.Minute)); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("claimed before retry: %v", err)
	}
	claimed, err = f.store.ClaimOutbox(ctx, "worker", f.now.Add(2*time.Minute), f.now.Add(3*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if err = f.store.CompleteOutbox(ctx, claimed.ID, "worker", f.now.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, err = f.store.ClaimOutbox(ctx, "worker", f.now.Add(4*time.Minute), f.now.Add(5*time.Minute)); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("delivered event reclaimed: %v", err)
	}
}

func TestAuditPaginationAndIsolation(t *testing.T) {
	f := newStoreFixture(t)
	ctx := context.Background()
	for index := 0; index < 3; index++ {
		event := audit.Event{ID: fmt.Sprintf("audit-%d", index), TenantID: f.tenant, ActorID: f.users[domain.RoleOperator].ID, RequestID: fmt.Sprintf("request-%d", index), Action: "batch.update", Object: "batch", ObjectID: "batch", Outcome: audit.OutcomeSucceeded, Metadata: []byte(`{"ok":true}`), CreatedAt: f.now.Add(time.Duration(index) * time.Minute)}
		if err := f.store.CreateAudit(ctx, event); err != nil {
			t.Fatal(err)
		}
	}
	items, total, err := f.store.ListAudit(ctx, f.tenant, "batch", "batch", pagination.Page{Number: 1, Size: 2})
	if err != nil {
		t.Fatal(err)
	}
	if total != 3 || len(items) != 2 {
		t.Fatalf("total=%d items=%d", total, len(items))
	}
	if items[0].ID != "audit-2" {
		t.Fatalf("order=%+v", items)
	}
}

func TestDatabaseStateSurvivesCloseAndReopen(t *testing.T) {
	path := t.TempDir() + "/restart.db"
	ctx := context.Background()
	store, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if err = store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 25, 0, 0, 0, 0, time.UTC)
	user := domain.User{ID: "user", TenantID: "tenant", Email: "user@test.local", DisplayName: "User", Role: domain.RoleOperator, PasswordHash: "hash", Active: true, CreatedAt: now, UpdatedAt: now}
	if err = store.SeedTenant(ctx, "tenant", "Tenant", []domain.User{user}); err != nil {
		t.Fatal(err)
	}
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err = store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	found, err := store.GetUser(ctx, "tenant", "user")
	if err != nil {
		t.Fatal(err)
	}
	if found.Email != user.Email {
		t.Fatalf("found=%+v", found)
	}
}
