package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/VanceMichael/go-label-yanji-mushroomchain-g12-v1/internal/clock"
	"github.com/VanceMichael/go-label-yanji-mushroomchain-g12-v1/internal/domain"
	"github.com/VanceMichael/go-label-yanji-mushroomchain-g12-v1/internal/pagination"
	"github.com/VanceMichael/go-label-yanji-mushroomchain-g12-v1/internal/repository"
	"github.com/VanceMichael/go-label-yanji-mushroomchain-g12-v1/internal/testkit"
)

type serviceFixture struct {
	data        testkit.Fixture
	clock       *clock.Fixed
	auth        *AuthService
	batches     *BatchService
	orders      *OrderService
	settlements *SettlementService
}

func newServiceFixture(t *testing.T) serviceFixture {
	t.Helper()
	data := testkit.New(t)
	fixed := clock.NewFixed(data.Now)
	return serviceFixture{data: data, clock: fixed, auth: NewAuth(data.Store, fixed, time.Hour), batches: NewBatches(data.Store, data.Store, fixed), orders: NewOrders(data.Store, data.Store, fixed), settlements: NewSettlements(data.Store, data.Store, fixed)}
}

func (f serviceFixture) ctx(role domain.Role) context.Context {
	user := f.data.Users[role]
	farmID := user.FarmID
	if role == domain.RoleFarmer {
		farmID = f.data.Farm.ID
	}
	return WithIdentity(context.Background(), Identity{UserID: user.ID, TenantID: user.TenantID, FarmID: farmID, SessionID: "session", Role: role})
}

func TestIdentityAndRoleAuthorization(t *testing.T) {
	f := newServiceFixture(t)
	if _, err := IdentityFrom(context.Background()); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatalf("anonymous identity error=%v", err)
	}
	ctx := f.ctx(domain.RoleInspector)
	identity, err := IdentityFrom(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if identity.Role != domain.RoleInspector {
		t.Fatalf("identity=%+v", identity)
	}
	if _, err = RequireRoles(ctx, domain.RoleInspector, domain.RoleOperator); err != nil {
		t.Fatalf("allowed role: %v", err)
	}
	if _, err = RequireRoles(ctx, domain.RoleFinance); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("forbidden role error=%v", err)
	}
}

func TestLoginAuthenticateLogoutLifecycle(t *testing.T) {
	f := newServiceFixture(t)
	result, err := f.auth.Login(context.Background(), LoginInput{TenantID: f.data.TenantID, Email: f.data.Users[domain.RoleOperator].Email, Password: f.data.Password})
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if result.Token == "" || !result.ExpiresAt.Equal(f.data.Now.Add(time.Hour)) {
		t.Fatalf("result=%+v", result)
	}
	identity, err := f.auth.Authenticate(context.Background(), result.Token)
	if err != nil {
		t.Fatalf("authenticate: %v", err)
	}
	if identity.UserID != result.User.ID || identity.Role != domain.RoleOperator {
		t.Fatalf("identity=%+v", identity)
	}
	ctx := WithIdentity(context.Background(), identity)
	if err = f.auth.Logout(ctx); err != nil {
		t.Fatalf("logout: %v", err)
	}
	if _, err = f.auth.Authenticate(context.Background(), result.Token); !errors.Is(err, domain.ErrExpired) {
		t.Fatalf("revoked authenticate error=%v", err)
	}
}

func TestLoginRejectsWrongPasswordAndUnknownUser(t *testing.T) {
	f := newServiceFixture(t)
	tests := []LoginInput{{TenantID: f.data.TenantID, Email: f.data.Users[domain.RoleOperator].Email, Password: "wrong-password-value"}, {TenantID: f.data.TenantID, Email: "unknown@test.local", Password: f.data.Password}, {TenantID: "", Email: "", Password: ""}}
	for index, input := range tests {
		_, err := f.auth.Login(context.Background(), input)
		if index < 2 && !errors.Is(err, domain.ErrUnauthorized) {
			t.Fatalf("case %d error=%v", index, err)
		}
		if index == 2 && !errors.Is(err, domain.ErrInvalid) {
			t.Fatalf("invalid case error=%v", err)
		}
	}
}

func TestAuthenticateRejectsExpiredSession(t *testing.T) {
	f := newServiceFixture(t)
	result, err := f.auth.Login(context.Background(), LoginInput{TenantID: f.data.TenantID, Email: f.data.Users[domain.RoleOperator].Email, Password: f.data.Password})
	if err != nil {
		t.Fatal(err)
	}
	f.clock.Advance(time.Hour)
	if _, err = f.auth.Authenticate(context.Background(), result.Token); !errors.Is(err, domain.ErrExpired) {
		t.Fatalf("expired authentication error=%v", err)
	}
}

func TestRegisterBatchEnforcesFarmOwnershipAndPersistsAudit(t *testing.T) {
	f := newServiceFixture(t)
	input := RegisterBatchInput{FarmID: f.data.Farm.ID, Code: "BATCH-REGISTER", Species: "oyster", ProducedAt: f.data.Now.Add(-time.Hour), ExpiresAt: f.data.Now.Add(30 * 24 * time.Hour), Quantity: 500, UnitPriceCents: 320, RequestID: "request-register"}
	batch, err := f.batches.Register(f.ctx(domain.RoleOperator), input)
	if err != nil {
		t.Fatalf("operator register: %v", err)
	}
	if batch.Status != domain.BatchRegistered || batch.QuantityAvailable != 500 {
		t.Fatalf("batch=%+v", batch)
	}
	stored, err := f.data.Store.GetBatch(context.Background(), f.data.TenantID, batch.ID)
	if err != nil || stored.Code != input.Code {
		t.Fatalf("stored=%+v err=%v", stored, err)
	}
	events, total, err := f.data.Store.ListAudit(context.Background(), f.data.TenantID, "batch", batch.ID, pagination.Page{Number: 1, Size: 20})
	if err != nil || total != 1 || events[0].RequestID != "request-register" {
		t.Fatalf("audit=%+v total=%d err=%v", events, total, err)
	}
	farmerCtx := WithIdentity(context.Background(), Identity{UserID: f.data.Users[domain.RoleFarmer].ID, TenantID: f.data.TenantID, FarmID: "another-farm", Role: domain.RoleFarmer})
	input.Code = "OTHER"
	if _, err = f.batches.Register(farmerCtx, input); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("foreign farm register error=%v", err)
	}
}

func TestInspectBatchTransitionsAtomically(t *testing.T) {
	f := newServiceFixture(t)
	batch := f.data.Batch(domain.BatchRegistered, 100)
	if err := f.data.Store.CreateBatch(context.Background(), batch); err != nil {
		t.Fatal(err)
	}
	released, err := f.batches.Inspect(f.ctx(domain.RoleInspector), InspectBatchInput{BatchID: batch.ID, Decision: domain.InspectionApproved, MoistureBP: 6100, SampleCount: 10, Notes: "quality accepted", RequestID: "inspect-request", ExpectedVersion: 1})
	if err != nil {
		t.Fatalf("inspect: %v", err)
	}
	if released.Status != domain.BatchReleased || released.Version != 3 {
		t.Fatalf("released=%+v", released)
	}
	inspections, err := f.data.Store.ListInspections(context.Background(), f.data.TenantID, batch.ID)
	if err != nil || len(inspections) != 1 {
		t.Fatalf("inspections=%+v err=%v", inspections, err)
	}
	if _, err = f.batches.Inspect(f.ctx(domain.RoleInspector), InspectBatchInput{BatchID: batch.ID, Decision: domain.InspectionRejected, MoistureBP: 9000, SampleCount: 2, ExpectedVersion: 1}); !errors.Is(err, domain.ErrVersionConflict) {
		t.Fatalf("stale inspection error=%v", err)
	}
	inspections, _ = f.data.Store.ListInspections(context.Background(), f.data.TenantID, batch.ID)
	if len(inspections) != 1 {
		t.Fatalf("stale inspection persisted: %+v", inspections)
	}
}

func TestInspectBatchRequiresInspectorRole(t *testing.T) {
	f := newServiceFixture(t)
	batch := f.data.Batch(domain.BatchRegistered, 100)
	if err := f.data.Store.CreateBatch(context.Background(), batch); err != nil {
		t.Fatal(err)
	}
	if _, err := f.batches.Inspect(f.ctx(domain.RoleFarmer), InspectBatchInput{BatchID: batch.ID, Decision: domain.InspectionApproved, MoistureBP: 6000, SampleCount: 5, ExpectedVersion: 1}); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("farmer inspect error=%v", err)
	}
}

func TestCreateOrderIsIdempotentAndConfirmed(t *testing.T) {
	f := newServiceFixture(t)
	input := CreateOrderInput{BuyerName: "Minning Buyer", DeliveryRegion: "Yinchuan", IdempotencyKey: "order-request", RequestID: "request-order", DueAt: f.data.Now.Add(24 * time.Hour), Lines: []domain.OrderLine{{Species: "oyster", Quantity: 12, UnitPriceCents: 400}}}
	first, err := f.orders.Create(f.ctx(domain.RoleDispatcher), input)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	second, err := f.orders.Create(f.ctx(domain.RoleDispatcher), input)
	if err != nil {
		t.Fatalf("idempotent create: %v", err)
	}
	if first.ID != second.ID {
		t.Fatalf("idempotent ids %s %s", first.ID, second.ID)
	}
	if first.Status != domain.OrderConfirmed || first.Version != 2 || first.TotalCents != 4800 {
		t.Fatalf("order=%+v", first)
	}
	items, total, err := f.data.Store.ListOrders(context.Background(), repository.OrderFilter{TenantID: f.data.TenantID, Page: pagination.Page{Number: 1, Size: 20}})
	if err != nil || total != 1 || len(items) != 1 {
		t.Fatalf("orders=%+v total=%d err=%v", items, total, err)
	}
}

func TestAllocateOrderRollsBackWhenCapacityIsInsufficient(t *testing.T) {
	f := newServiceFixture(t)
	batch := f.data.Batch(domain.BatchReleased, 5)
	if err := f.data.Store.CreateBatch(context.Background(), batch); err != nil {
		t.Fatal(err)
	}
	order, err := f.orders.Create(f.ctx(domain.RoleDispatcher), CreateOrderInput{BuyerName: "Buyer", DeliveryRegion: "Yinchuan", IdempotencyKey: "insufficient", DueAt: f.data.Now.Add(time.Hour), Lines: []domain.OrderLine{{Species: "oyster", Quantity: 8, UnitPriceCents: 400}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.orders.Allocate(f.ctx(domain.RoleDispatcher), AllocateInput{OrderID: order.ID, ExpectedVersion: order.Version, RequestID: "allocate"}); !errors.Is(err, domain.ErrCapacity) {
		t.Fatalf("allocate error=%v", err)
	}
	stored, err := f.data.Store.GetBatch(context.Background(), f.data.TenantID, batch.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.QuantityAvailable != 5 || stored.Version != 1 {
		t.Fatalf("batch leaked allocation: %+v", stored)
	}
	allocations, err := f.data.Store.ListAllocations(context.Background(), f.data.TenantID, order.ID)
	if err != nil || len(allocations) != 0 {
		t.Fatalf("allocations=%+v err=%v", allocations, err)
	}
	storedOrder, _ := f.data.Store.GetOrder(context.Background(), f.data.TenantID, order.ID)
	if storedOrder.Status != domain.OrderConfirmed {
		t.Fatalf("order changed=%+v", storedOrder)
	}
}

func TestAllocateOrderConsumesEarliestExpiringBatchFirst(t *testing.T) {
	f := newServiceFixture(t)
	later := f.data.Batch(domain.BatchReleased, 10)
	later.ID = "batch-later-expiry"
	later.Code = "BATCH-LATER-EXPIRY"
	later.ProducedAt = f.data.Now.Add(-time.Hour)
	later.ExpiresAt = f.data.Now.Add(10 * 24 * time.Hour)
	earlier := f.data.Batch(domain.BatchReleased, 10)
	earlier.ID = "batch-earlier-expiry"
	earlier.Code = "BATCH-EARLIER-EXPIRY"
	earlier.ProducedAt = f.data.Now.Add(-48 * time.Hour)
	earlier.ExpiresAt = f.data.Now.Add(2 * 24 * time.Hour)
	for _, batch := range []domain.SubstrateBatch{later, earlier} {
		if err := f.data.Store.CreateBatch(context.Background(), batch); err != nil {
			t.Fatal(err)
		}
	}
	order, err := f.orders.Create(f.ctx(domain.RoleDispatcher), CreateOrderInput{BuyerName: "Buyer", DeliveryRegion: "Yinchuan", IdempotencyKey: "expiry-order", DueAt: f.data.Now.Add(time.Hour), Lines: []domain.OrderLine{{Species: "oyster", Quantity: 6, UnitPriceCents: 400}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.orders.Allocate(f.ctx(domain.RoleDispatcher), AllocateInput{OrderID: order.ID, ExpectedVersion: order.Version, RequestID: "allocate-expiry-order"}); err != nil {
		t.Fatalf("allocate: %v", err)
	}
	earlierStored, err := f.data.Store.GetBatch(context.Background(), f.data.TenantID, earlier.ID)
	if err != nil {
		t.Fatal(err)
	}
	laterStored, err := f.data.Store.GetBatch(context.Background(), f.data.TenantID, later.ID)
	if err != nil {
		t.Fatal(err)
	}
	allocations, err := f.data.Store.ListAllocations(context.Background(), f.data.TenantID, order.ID)
	if err != nil {
		t.Fatal(err)
	}
	if earlierStored.QuantityAvailable != 4 || laterStored.QuantityAvailable != 10 || len(allocations) != 1 || allocations[0].BatchID != earlier.ID {
		t.Fatalf("earlier=%+v later=%+v allocations=%+v", earlierStored, laterStored, allocations)
	}
}

func TestAllocateAndDeliverCreatesFarmSettlement(t *testing.T) {
	f := newServiceFixture(t)
	batch := f.data.Batch(domain.BatchReleased, 20)
	if err := f.data.Store.CreateBatch(context.Background(), batch); err != nil {
		t.Fatal(err)
	}
	order, err := f.orders.Create(f.ctx(domain.RoleDispatcher), CreateOrderInput{BuyerName: "Buyer", DeliveryRegion: "Yinchuan", IdempotencyKey: "complete-flow", DueAt: f.data.Now.Add(time.Hour), Lines: []domain.OrderLine{{Species: "oyster", Quantity: 8, UnitPriceCents: 400}}})
	if err != nil {
		t.Fatal(err)
	}
	allocated, err := f.orders.Allocate(f.ctx(domain.RoleDispatcher), AllocateInput{OrderID: order.ID, ExpectedVersion: order.Version, RequestID: "allocate"})
	if err != nil {
		t.Fatalf("allocate: %v", err)
	}
	if allocated.Status != domain.OrderAllocated || allocated.Version != 3 {
		t.Fatalf("allocated=%+v", allocated)
	}
	storedBatch, _ := f.data.Store.GetBatch(context.Background(), f.data.TenantID, batch.ID)
	if storedBatch.QuantityAvailable != 12 {
		t.Fatalf("available=%d", storedBatch.QuantityAvailable)
	}
	delivered, err := f.orders.MarkDelivered(f.ctx(domain.RoleDispatcher), DeliveryInput{OrderID: order.ID, ExpectedVersion: allocated.Version, RequestID: "deliver"})
	if err != nil {
		t.Fatalf("deliver: %v", err)
	}
	if delivered.Status != domain.OrderDelivered || delivered.Version != 5 {
		t.Fatalf("delivered=%+v", delivered)
	}
	settlements, err := f.settlements.ListForOrder(f.ctx(domain.RoleFinance), order.ID)
	if err != nil || len(settlements) != 1 {
		t.Fatalf("settlements=%+v err=%v", settlements, err)
	}
	if settlements[0].FarmID != f.data.Farm.ID || settlements[0].AmountCents != 8*batch.UnitPriceCents {
		t.Fatalf("settlement=%+v", settlements[0])
	}
}

func TestSettlementApprovalAndPaymentRequireFinance(t *testing.T) {
	f := newServiceFixture(t)
	order := f.data.Order(domain.OrderDelivered, 5)
	if err := f.data.Store.CreateOrder(context.Background(), order); err != nil {
		t.Fatal(err)
	}
	item := domain.Settlement{ID: "settlement", TenantID: f.data.TenantID, FarmID: f.data.Farm.ID, OrderID: order.ID, AmountCents: 1500, Status: domain.SettlementPending, Version: 1, CreatedAt: f.data.Now, UpdatedAt: f.data.Now}
	if err := f.data.Store.CreateSettlement(context.Background(), item); err != nil {
		t.Fatal(err)
	}
	if _, err := f.settlements.Approve(f.ctx(domain.RoleFarmer), ApproveSettlementInput{SettlementID: item.ID, ExpectedVersion: 1}); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("farmer approve error=%v", err)
	}
	approved, err := f.settlements.Approve(f.ctx(domain.RoleFinance), ApproveSettlementInput{SettlementID: item.ID, ExpectedVersion: 1, RequestID: "approve"})
	if err != nil {
		t.Fatal(err)
	}
	if approved.Status != domain.SettlementApproved || approved.Version != 2 {
		t.Fatalf("approved=%+v", approved)
	}
	paid, err := f.settlements.MarkPaid(f.ctx(domain.RoleFinance), PaySettlementInput{SettlementID: item.ID, ExpectedVersion: 2, RequestID: "pay"})
	if err != nil {
		t.Fatal(err)
	}
	if paid.Status != domain.SettlementPaid || paid.Version != 3 || paid.PaidAt == nil {
		t.Fatalf("paid=%+v", paid)
	}
}

func TestFarmerSeesOnlyOwnSettlements(t *testing.T) {
	f := newServiceFixture(t)
	order := f.data.Order(domain.OrderDelivered, 5)
	if err := f.data.Store.CreateOrder(context.Background(), order); err != nil {
		t.Fatal(err)
	}
	item := domain.Settlement{ID: "settlement", TenantID: f.data.TenantID, FarmID: "different-farm", OrderID: order.ID, AmountCents: 1500, Status: domain.SettlementPending, Version: 1, CreatedAt: f.data.Now, UpdatedAt: f.data.Now}
	if err := f.data.Store.CreateSettlement(context.Background(), item); err == nil {
		t.Fatal("foreign key unexpectedly accepted missing farm")
	}
	own := item
	own.ID = "own"
	own.FarmID = f.data.Farm.ID
	if err := f.data.Store.CreateSettlement(context.Background(), own); err != nil {
		t.Fatal(err)
	}
	items, err := f.settlements.ListForOrder(f.ctx(domain.RoleFarmer), order.ID)
	if err != nil || len(items) != 1 || items[0].ID != "own" {
		t.Fatalf("items=%+v err=%v", items, err)
	}
}
