package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/VanceMichael/go-label-yanji-mushroomchain-g12-v1/internal/audit"
	"github.com/VanceMichael/go-label-yanji-mushroomchain-g12-v1/internal/domain"
	"github.com/VanceMichael/go-label-yanji-mushroomchain-g12-v1/internal/pagination"
	"github.com/VanceMichael/go-label-yanji-mushroomchain-g12-v1/internal/repository"
	"github.com/VanceMichael/go-label-yanji-mushroomchain-g12-v1/internal/testkit"
)

var errInjectedAuditFailure = errors.New("audit write unavailable")

type failingAuditTransactor struct {
	repository.Transactor
}

func (w failingAuditTransactor) WithinTx(ctx context.Context, fn func(repository.Tx) error) error {
	return w.Transactor.WithinTx(ctx, func(tx repository.Tx) error {
		return fn(failingAuditTx{Tx: tx})
	})
}

type failingAuditTx struct {
	repository.Tx
}

func (w failingAuditTx) CreateAudit(context.Context, audit.Event) error {
	return errInjectedAuditFailure
}

type failingAuditRepo struct {
	repository.Tx
}

func (failingAuditRepo) CreateAudit(context.Context, audit.Event) error {
	return errInjectedAuditFailure
}

func TestOrderCreateAuditFailureRollsBackOrderAndLines(t *testing.T) {
	fixture := testkit.New(t)
	ctx := WithIdentity(context.Background(), Identity{UserID: fixture.Users[domain.RoleDispatcher].ID, TenantID: fixture.TenantID, Role: domain.RoleDispatcher})
	service := NewOrders(failingAuditTransactor{Transactor: fixture.Store}, failingAuditRepo{Tx: fixture.Store}, testkitClock(fixture.Now))
	input := CreateOrderInput{
		BuyerName: "Audit Failure Buyer", DeliveryRegion: "Yinchuan", IdempotencyKey: "audit-rollback-0013", RequestID: "audit-failure-0013",
		DueAt: fixture.Now.Add(24 * time.Hour),
		Lines: []domain.OrderLine{{Species: "oyster", Quantity: 4, UnitPriceCents: 420}, {Species: "shiitake", Quantity: 3, UnitPriceCents: 510}},
	}
	if _, err := service.Create(ctx, input); !errors.Is(err, errInjectedAuditFailure) {
		t.Fatalf("create error=%v, want injected audit failure", err)
	}
	if _, err := fixture.Store.FindOrderByIdempotency(context.Background(), fixture.TenantID, input.IdempotencyKey); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("failed order still queryable: order error=%v", err)
	}
	orders, total, err := fixture.Store.ListOrders(context.Background(), repository.OrderFilter{TenantID: fixture.TenantID, Page: pagination.Page{Number: 1, Size: 20}})
	if err != nil || total != 0 || len(orders) != 0 {
		t.Fatalf("failed create left orders=%+v total=%d err=%v", orders, total, err)
	}

	retryService := NewOrders(fixture.Store, fixture.Store, testkitClock(fixture.Now))
	retryCtx := WithIdentity(context.Background(), Identity{UserID: fixture.Users[domain.RoleDispatcher].ID, TenantID: fixture.TenantID, Role: domain.RoleDispatcher})
	created, err := retryService.Create(retryCtx, input)
	if err != nil {
		t.Fatalf("retry create: %v", err)
	}
	if created.Status != domain.OrderConfirmed || len(created.Lines) != 2 {
		t.Fatalf("retry order=%+v", created)
	}
	if _, total, err = fixture.Store.ListAudit(context.Background(), fixture.TenantID, "order", created.ID, pagination.Page{Number: 1, Size: 20}); err != nil || total != 1 {
		t.Fatalf("audit records=%d err=%v", total, err)
	}
}

func testkitClock(now time.Time) *fixedClockAdapter { return &fixedClockAdapter{now: now} }

type fixedClockAdapter struct{ now time.Time }

func (c *fixedClockAdapter) Now() time.Time { return c.now }
