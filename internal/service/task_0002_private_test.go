package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/VanceMichael/go-label-yanji-mushroomchain-g12-v1/internal/audit"
	"github.com/VanceMichael/go-label-yanji-mushroomchain-g12-v1/internal/clock"
	"github.com/VanceMichael/go-label-yanji-mushroomchain-g12-v1/internal/domain"
	"github.com/VanceMichael/go-label-yanji-mushroomchain-g12-v1/internal/pagination"
	"github.com/VanceMichael/go-label-yanji-mushroomchain-g12-v1/internal/repository"
	"github.com/VanceMichael/go-label-yanji-mushroomchain-g12-v1/internal/testkit"
)

var errTask0002AuditRejected = errors.New("audit repository rejected event")

type task0002RejectingAuditTx struct {
	repository.Tx
}

func (task0002RejectingAuditTx) CreateAudit(context.Context, audit.Event) error {
	return errTask0002AuditRejected
}

type task0002RejectingAuditTransactor struct {
	repository.Transactor
}

func (t task0002RejectingAuditTransactor) WithinTx(ctx context.Context, fn func(repository.Tx) error) error {
	return t.Transactor.WithinTx(ctx, func(tx repository.Tx) error {
		return fn(task0002RejectingAuditTx{Tx: tx})
	})
}

func TestRegisterBatchRollsBackWhenAuditRejected(t *testing.T) {
	data := testkit.New(t)
	fixedClock := clock.NewFixed(data.Now)
	operator := data.Users[domain.RoleOperator]
	ctx := WithIdentity(context.Background(), Identity{
		UserID: operator.ID, TenantID: operator.TenantID, Role: operator.Role,
	})
	input := RegisterBatchInput{
		FarmID: data.Farm.ID, Code: "AUDIT-ROLLBACK-0002", Species: "oyster",
		ProducedAt: data.Now.Add(-time.Hour), ExpiresAt: data.Now.Add(30 * 24 * time.Hour),
		Quantity: 240, UnitPriceCents: 360, RequestID: "request-audit-rejected",
	}

	failingService := NewBatches(
		task0002RejectingAuditTransactor{Transactor: data.Store}, data.Store, fixedClock,
	)
	if _, err := failingService.Register(ctx, input); !errors.Is(err, errTask0002AuditRejected) {
		t.Fatalf("register error = %v, want audit rejection", err)
	}
	items, total, err := data.Store.ListBatches(context.Background(), repository.BatchFilter{
		TenantID: data.TenantID, Page: pagination.Page{Number: 1, Size: 20},
	})
	if err != nil {
		t.Fatalf("list batches after rejected audit: %v", err)
	}
	if total != 0 || len(items) != 0 {
		t.Errorf("rejected registration persisted batches: total=%d items=%+v", total, items)
	}

	input.RequestID = "request-audit-retry"
	retryService := NewBatches(data.Store, data.Store, fixedClock)
	created, err := retryService.Register(ctx, input)
	if err != nil {
		t.Fatalf("retry same batch code after audit recovery: %v", err)
	}
	if created.Code != input.Code || created.Status != domain.BatchRegistered {
		t.Fatalf("retried batch = %+v", created)
	}
	items, total, err = data.Store.ListBatches(context.Background(), repository.BatchFilter{
		TenantID: data.TenantID, Page: pagination.Page{Number: 1, Size: 20},
	})
	if err != nil || total != 1 || len(items) != 1 || items[0].Code != input.Code {
		t.Fatalf("batches after retry: total=%d items=%+v err=%v", total, items, err)
	}
	events, auditTotal, err := data.Store.ListAudit(
		context.Background(), data.TenantID, "batch", created.ID, pagination.Page{Number: 1, Size: 20},
	)
	if err != nil || auditTotal != 1 || len(events) != 1 || events[0].RequestID != input.RequestID {
		t.Fatalf("audit after retry: total=%d events=%+v err=%v", auditTotal, events, err)
	}
}
