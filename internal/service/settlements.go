package service

import (
	"context"
	"encoding/json"
	"time"

	"github.com/VanceMichael/go-label-yanji-mushroomchain-g12-v1/internal/audit"
	"github.com/VanceMichael/go-label-yanji-mushroomchain-g12-v1/internal/auth"
	"github.com/VanceMichael/go-label-yanji-mushroomchain-g12-v1/internal/clock"
	"github.com/VanceMichael/go-label-yanji-mushroomchain-g12-v1/internal/domain"
	"github.com/VanceMichael/go-label-yanji-mushroomchain-g12-v1/internal/repository"
)

type SettlementService struct {
	tx    repository.Transactor
	repo  repository.Tx
	clock clock.Clock
}

func NewSettlements(tx repository.Transactor, repo repository.Tx, c clock.Clock) *SettlementService {
	return &SettlementService{tx: tx, repo: repo, clock: c}
}

func (s *SettlementService) ListForOrder(ctx context.Context, orderID string) ([]domain.Settlement, error) {
	actor, err := IdentityFrom(ctx)
	if err != nil {
		return nil, err
	}
	items, err := s.repo.ListSettlementsByOrder(ctx, actor.TenantID, orderID)
	if err != nil {
		return nil, err
	}
	if actor.Role == domain.RoleFarmer {
		filtered := items[:0]
		for _, item := range items {
			if item.FarmID == actor.FarmID {
				filtered = append(filtered, item)
			}
		}
		items = filtered
	}
	return items, nil
}

type ApproveSettlementInput struct {
	SettlementID, RequestID string
	ExpectedVersion         int64
}

func (s *SettlementService) Approve(ctx context.Context, input ApproveSettlementInput) (domain.Settlement, error) {
	actor, err := RequireRoles(ctx, domain.RoleFinance)
	if err != nil {
		return domain.Settlement{}, err
	}
	now := s.clock.Now()
	err = s.tx.WithinTx(ctx, func(tx repository.Tx) error {
		item, e := tx.GetSettlement(ctx, actor.TenantID, input.SettlementID)
		if e != nil {
			return e
		}
		if item.Status != domain.SettlementPending {
			return domain.StateError{Entity: "settlement", From: string(item.Status), To: string(domain.SettlementApproved)}
		}
		if item.Version != input.ExpectedVersion {
			return domain.ErrVersionConflict
		}
		if e = tx.ApproveSettlement(ctx, actor.TenantID, item.ID, item.Version, actor.UserID, now); e != nil {
			return e
		}
		payload, _ := json.Marshal(map[string]any{"settlement_id": item.ID, "approved_by": actor.UserID, "amount_cents": item.AmountCents})
		eventID, e := auth.NewID("evt")
		if e != nil {
			return e
		}
		if e = tx.CreateOutboxEvent(ctx, domain.OutboxEvent{ID: eventID, TenantID: actor.TenantID, AggregateType: "settlement", AggregateID: item.ID, EventType: "settlement.approved", Payload: payload, Status: domain.OutboxPending, AvailableAt: now, CreatedAt: now}); e != nil {
			return e
		}
		auditEvent, e := newAudit(actor, input.RequestID, "settlement.approve", "settlement", item.ID, audit.OutcomeSucceeded, map[string]any{"amount_cents": item.AmountCents}, now)
		if e != nil {
			return e
		}
		return tx.CreateAudit(ctx, auditEvent)
	})
	if err != nil {
		return domain.Settlement{}, err
	}
	return s.repo.GetSettlement(ctx, actor.TenantID, input.SettlementID)
}

type PaySettlementInput struct {
	SettlementID, RequestID string
	ExpectedVersion         int64
}

func (s *SettlementService) MarkPaid(ctx context.Context, input PaySettlementInput) (domain.Settlement, error) {
	actor, err := RequireRoles(ctx, domain.RoleFinance)
	if err != nil {
		return domain.Settlement{}, err
	}
	now := s.clock.Now()
	err = s.tx.WithinTx(ctx, func(tx repository.Tx) error {
		item, e := tx.GetSettlement(ctx, actor.TenantID, input.SettlementID)
		if e != nil {
			return e
		}
		if item.Status != domain.SettlementApproved {
			return domain.StateError{Entity: "settlement", From: string(item.Status), To: string(domain.SettlementPaid)}
		}
		if e = tx.MarkSettlementPaid(ctx, actor.TenantID, item.ID, input.ExpectedVersion, now); e != nil {
			return e
		}
		payload, _ := json.Marshal(map[string]any{"settlement_id": item.ID, "paid_at": now})
		eventID, e := auth.NewID("evt")
		if e != nil {
			return e
		}
		if e = tx.CreateOutboxEvent(ctx, domain.OutboxEvent{ID: eventID, TenantID: actor.TenantID, AggregateType: "settlement", AggregateID: item.ID, EventType: "settlement.paid", Payload: payload, Status: domain.OutboxPending, AvailableAt: now, CreatedAt: now}); e != nil {
			return e
		}
		auditEvent, e := newAudit(actor, input.RequestID, "settlement.pay", "settlement", item.ID, audit.OutcomeSucceeded, map[string]any{"amount_cents": item.AmountCents}, now)
		if e != nil {
			return e
		}
		return tx.CreateAudit(ctx, auditEvent)
	})
	if err != nil {
		return domain.Settlement{}, err
	}
	return s.repo.GetSettlement(ctx, actor.TenantID, input.SettlementID)
}

var _ time.Time
