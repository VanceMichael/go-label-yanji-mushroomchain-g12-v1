package service

import (
	"context"
	"errors"
	"testing"

	"github.com/VanceMichael/go-label-yanji-mushroomchain-g12-v1/internal/domain"
	"github.com/VanceMichael/go-label-yanji-mushroomchain-g12-v1/internal/pagination"
)

func TestSettlementApprovalFailureIsAtomic(t *testing.T) {
	f := newServiceFixture(t)
	ctx := context.Background()
	order := f.data.Order(domain.OrderDelivered, 5)
	if err := f.data.Store.CreateOrder(ctx, order); err != nil {
		t.Fatalf("create delivered order: %v", err)
	}
	settlement := domain.Settlement{
		ID:          "settlement-atomic-approval",
		TenantID:    f.data.TenantID,
		FarmID:      f.data.Farm.ID,
		OrderID:     order.ID,
		AmountCents: 2400,
		Status:      domain.SettlementPending,
		Version:     1,
		CreatedAt:   f.data.Now,
		UpdatedAt:   f.data.Now,
	}
	if err := f.data.Store.CreateSettlement(ctx, settlement); err != nil {
		t.Fatalf("create pending settlement: %v", err)
	}

	missingFinance := WithIdentity(ctx, Identity{
		UserID:    "missing-finance-user",
		TenantID:  f.data.TenantID,
		SessionID: "missing-finance-session",
		Role:      domain.RoleFinance,
	})
	if _, err := f.settlements.Approve(missingFinance, ApproveSettlementInput{
		SettlementID:    settlement.ID,
		ExpectedVersion: settlement.Version,
		RequestID:       "approval-fails-at-audit",
	}); err == nil {
		t.Fatal("approval unexpectedly succeeded when its audit actor was rejected")
	}

	stored, err := f.data.Store.GetSettlement(ctx, f.data.TenantID, settlement.ID)
	if err != nil {
		t.Fatalf("read settlement after failed approval: %v", err)
	}
	if stored.Status != domain.SettlementPending || stored.Version != settlement.Version || stored.ApprovedBy != "" || stored.ApprovedAt != nil {
		t.Fatalf("failed approval changed settlement: %+v", stored)
	}
	if event, err := f.data.Store.ClaimOutbox(ctx, "approval-check", f.data.Now, f.data.Now); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("failed approval left an outbox event: event=%+v err=%v", event, err)
	}
	audits, total, err := f.data.Store.ListAudit(ctx, f.data.TenantID, "settlement", settlement.ID, pagination.Page{Number: 1, Size: 20})
	if err != nil || total != 0 || len(audits) != 0 {
		t.Fatalf("failed approval left audit records: audits=%+v total=%d err=%v", audits, total, err)
	}

	approved, err := f.settlements.Approve(f.ctx(domain.RoleFinance), ApproveSettlementInput{
		SettlementID:    settlement.ID,
		ExpectedVersion: settlement.Version,
		RequestID:       "approval-retry",
	})
	if err != nil {
		t.Fatalf("retry approval with the original version: %v", err)
	}
	if approved.Status != domain.SettlementApproved || approved.Version != settlement.Version+1 {
		t.Fatalf("retry did not approve settlement: %+v", approved)
	}
	event, err := f.data.Store.ClaimOutbox(ctx, "approval-check", f.data.Now, f.data.Now)
	if err != nil || event.EventType != "settlement.approved" || event.AggregateID != settlement.ID {
		t.Fatalf("retry did not create approval event: event=%+v err=%v", event, err)
	}
	audits, total, err = f.data.Store.ListAudit(ctx, f.data.TenantID, "settlement", settlement.ID, pagination.Page{Number: 1, Size: 20})
	if err != nil || total != 1 || len(audits) != 1 || audits[0].Action != "settlement.approve" {
		t.Fatalf("retry did not create approval audit: audits=%+v total=%d err=%v", audits, total, err)
	}
}
