package service

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/VanceMichael/go-label-yanji-mushroomchain-g12-v1/internal/clock"
	"github.com/VanceMichael/go-label-yanji-mushroomchain-g12-v1/internal/domain"
	"github.com/VanceMichael/go-label-yanji-mushroomchain-g12-v1/internal/pagination"
	"github.com/VanceMichael/go-label-yanji-mushroomchain-g12-v1/internal/repository"
	"github.com/VanceMichael/go-label-yanji-mushroomchain-g12-v1/internal/testkit"
)

type settlementPaymentSnapshotBarrier struct {
	repository.Tx
	arrived chan<- struct{}
	release <-chan struct{}
}

func (b settlementPaymentSnapshotBarrier) GetSettlement(ctx context.Context, tenantID, settlementID string) (domain.Settlement, error) {
	item, err := b.Tx.GetSettlement(ctx, tenantID, settlementID)
	if err != nil {
		return domain.Settlement{}, err
	}
	if item.Status == domain.SettlementApproved && item.Version == 2 {
		b.arrived <- struct{}{}
		select {
		case <-b.release:
		case <-ctx.Done():
			return domain.Settlement{}, ctx.Err()
		}
	}
	return item, nil
}

func TestConcurrentSettlementPaymentEmitsOnePaidEvent(t *testing.T) {
	data := testkit.New(t)
	ctx := context.Background()
	order := data.Order(domain.OrderDelivered, 4)
	if err := data.Store.CreateOrder(ctx, order); err != nil {
		t.Fatalf("create delivered order: %v", err)
	}
	approvedAt := data.Now.Add(-time.Minute)
	settlement := domain.Settlement{
		ID: "settlement-concurrent-payment", TenantID: data.TenantID, FarmID: data.Farm.ID,
		OrderID: order.ID, AmountCents: 1600, Status: domain.SettlementApproved, Version: 2,
		ApprovedBy: data.Users[domain.RoleFinance].ID, ApprovedAt: &approvedAt,
		CreatedAt: data.Now.Add(-time.Hour), UpdatedAt: approvedAt,
	}
	if err := data.Store.CreateSettlement(ctx, settlement); err != nil {
		t.Fatalf("create approved settlement: %v", err)
	}

	arrived := make(chan struct{}, 2)
	release := make(chan struct{})
	repo := settlementPaymentSnapshotBarrier{Tx: data.Store, arrived: arrived, release: release}
	service := NewSettlements(data.Store, repo, clock.NewFixed(data.Now))
	finance := data.Users[domain.RoleFinance]
	financeCtx := WithIdentity(ctx, Identity{
		UserID: finance.ID, TenantID: finance.TenantID, FarmID: finance.FarmID,
		SessionID: "finance-session", Role: finance.Role,
	})

	results := make(chan error, 2)
	for request := 1; request <= 2; request++ {
		requestID := fmt.Sprintf("payment-%d", request)
		go func() {
			_, err := service.MarkPaid(financeCtx, PaySettlementInput{
				SettlementID: settlement.ID, ExpectedVersion: 2, RequestID: requestID,
			})
			results <- err
		}()
	}
	<-arrived
	<-arrived
	close(release)

	succeeded, conflicted := 0, 0
	for request := 0; request < 2; request++ {
		err := <-results
		switch {
		case err == nil:
			succeeded++
		case errors.Is(err, domain.ErrVersionConflict):
			conflicted++
		default:
			t.Fatalf("payment returned unexpected error: %v", err)
		}
	}
	if succeeded != 1 || conflicted != 1 {
		t.Fatalf("payment outcomes: succeeded=%d version_conflicts=%d", succeeded, conflicted)
	}

	stored, err := data.Store.GetSettlement(ctx, data.TenantID, settlement.ID)
	if err != nil {
		t.Fatalf("get paid settlement: %v", err)
	}
	if stored.Status != domain.SettlementPaid || stored.Version != 3 || stored.PaidAt == nil {
		t.Fatalf("settlement after concurrent payment: %+v", stored)
	}

	paidEvents := 0
	for probe := 1; ; probe++ {
		event, claimErr := data.Store.ClaimOutbox(ctx, fmt.Sprintf("probe-%d", probe), data.Now.Add(time.Second), data.Now.Add(time.Minute))
		if errors.Is(claimErr, domain.ErrNotFound) {
			break
		}
		if claimErr != nil {
			t.Fatalf("claim paid event: %v", claimErr)
		}
		if event.AggregateID != settlement.ID || event.EventType != "settlement.paid" {
			t.Fatalf("unexpected outbox event: %+v", event)
		}
		paidEvents++
	}
	if paidEvents != 1 {
		t.Fatalf("settlement.paid events=%d, want 1", paidEvents)
	}

	audits, total, err := data.Store.ListAudit(ctx, data.TenantID, "settlement", settlement.ID, pagination.Page{Number: 1, Size: 10})
	if err != nil {
		t.Fatalf("list payment audit: %v", err)
	}
	if total != 1 || len(audits) != 1 || audits[0].Action != "settlement.pay" {
		t.Fatalf("payment audits: total=%d items=%+v", total, audits)
	}
}
