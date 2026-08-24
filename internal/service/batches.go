package service

import (
	"context"
	"encoding/json"
	"time"

	"github.com/VanceMichael/go-label-yanji-mushroomchain-g12-v1/internal/audit"
	"github.com/VanceMichael/go-label-yanji-mushroomchain-g12-v1/internal/auth"
	"github.com/VanceMichael/go-label-yanji-mushroomchain-g12-v1/internal/clock"
	"github.com/VanceMichael/go-label-yanji-mushroomchain-g12-v1/internal/domain"
	"github.com/VanceMichael/go-label-yanji-mushroomchain-g12-v1/internal/pagination"
	"github.com/VanceMichael/go-label-yanji-mushroomchain-g12-v1/internal/repository"
)

type BatchService struct {
	tx    repository.Transactor
	repo  repository.Tx
	clock clock.Clock
}

func NewBatches(tx repository.Transactor, repo repository.Tx, c clock.Clock) *BatchService {
	return &BatchService{tx: tx, repo: repo, clock: c}
}

type RegisterBatchInput struct {
	FarmID, Code, Species string
	ProducedAt, ExpiresAt time.Time
	Quantity              int64
	UnitPriceCents        int64
	RequestID             string
}

func (s *BatchService) Register(ctx context.Context, input RegisterBatchInput) (domain.SubstrateBatch, error) {
	actor, err := RequireRoles(ctx, domain.RoleOperator, domain.RoleFarmer)
	if err != nil {
		return domain.SubstrateBatch{}, err
	}
	if actor.Role == domain.RoleFarmer && actor.FarmID != input.FarmID {
		return domain.SubstrateBatch{}, domain.ErrForbidden
	}
	id, err := auth.NewID("bat")
	if err != nil {
		return domain.SubstrateBatch{}, err
	}
	now := s.clock.Now()
	batch := domain.SubstrateBatch{ID: id, TenantID: actor.TenantID, FarmID: input.FarmID, Code: input.Code, Species: input.Species, ProducedAt: input.ProducedAt.UTC(), ExpiresAt: input.ExpiresAt.UTC(), QuantityProduced: input.Quantity, QuantityAvailable: input.Quantity, UnitPriceCents: input.UnitPriceCents, Status: domain.BatchRegistered, Version: 1, CreatedAt: now, UpdatedAt: now}
	if err := batch.Validate(); err != nil {
		return domain.SubstrateBatch{}, err
	}
	err = s.tx.WithinTx(ctx, func(tx repository.Tx) error {
		if _, e := tx.GetFarm(ctx, actor.TenantID, input.FarmID); e != nil {
			return e
		}
		if e := tx.CreateBatch(ctx, batch); e != nil {
			return e
		}
		event, e := newAudit(actor, input.RequestID, "batch.register", "batch", batch.ID, audit.OutcomeSucceeded, map[string]any{"code": batch.Code, "quantity": batch.QuantityProduced}, now)
		if e != nil {
			return e
		}
		return tx.CreateAudit(ctx, event)
	})
	if err != nil {
		return domain.SubstrateBatch{}, err
	}
	return batch, nil
}

type InspectBatchInput struct {
	BatchID                 string
	Decision                domain.InspectionDecision
	MoistureBP, SampleCount int
	Notes, RequestID        string
	ExpectedVersion         int64
}

func (s *BatchService) Inspect(ctx context.Context, input InspectBatchInput) (domain.SubstrateBatch, error) {
	actor, err := RequireRoles(ctx, domain.RoleInspector)
	if err != nil {
		return domain.SubstrateBatch{}, err
	}
	if input.SampleCount <= 0 || input.MoistureBP < 0 || input.MoistureBP > 10000 {
		return domain.SubstrateBatch{}, domain.ErrInvalid
	}
	if input.Decision != domain.InspectionApproved && input.Decision != domain.InspectionRejected {
		return domain.SubstrateBatch{}, domain.ErrInvalid
	}
	now := s.clock.Now()
	inspectionID, err := auth.NewID("qci")
	if err != nil {
		return domain.SubstrateBatch{}, err
	}
	err = s.tx.WithinTx(ctx, func(tx repository.Tx) error {
		batch, e := tx.GetBatch(ctx, actor.TenantID, input.BatchID)
		if e != nil {
			return e
		}
		if batch.Version != input.ExpectedVersion {
			return domain.ErrVersionConflict
		}
		if batch.Status == domain.BatchRegistered {
			if e = tx.TransitionBatch(ctx, actor.TenantID, batch.ID, batch.Version, domain.BatchSampling, now); e != nil {
				return e
			}
			batch.Version++
			batch.Status = domain.BatchSampling
		}
		if batch.Status != domain.BatchSampling {
			return domain.StateError{Entity: "batch", From: string(batch.Status), To: string(input.Decision)}
		}
		inspection := domain.QualityInspection{ID: inspectionID, TenantID: actor.TenantID, BatchID: batch.ID, InspectorID: actor.UserID, Decision: input.Decision, MoistureBP: input.MoistureBP, SampleCount: input.SampleCount, Notes: input.Notes, InspectedAt: now, CreatedAt: now}
		if e = tx.CreateInspection(ctx, inspection); e != nil {
			return e
		}
		target := domain.BatchReleased
		if input.Decision == domain.InspectionRejected {
			target = domain.BatchRejected
		}
		if e = tx.TransitionBatch(ctx, actor.TenantID, batch.ID, batch.Version, target, now); e != nil {
			return e
		}
		event, e := newAudit(actor, input.RequestID, "batch.inspect", "batch", batch.ID, audit.OutcomeSucceeded, map[string]any{"decision": input.Decision, "sample_count": input.SampleCount}, now)
		if e != nil {
			return e
		}
		return tx.CreateAudit(ctx, event)
	})
	if err != nil {
		return domain.SubstrateBatch{}, err
	}
	return s.repo.GetBatch(ctx, actor.TenantID, input.BatchID)
}

func (s *BatchService) Get(ctx context.Context, id string) (domain.SubstrateBatch, error) {
	actor, err := IdentityFrom(ctx)
	if err != nil {
		return domain.SubstrateBatch{}, err
	}
	batch, err := s.repo.GetBatch(ctx, actor.TenantID, id)
	if err != nil {
		return batch, err
	}
	if actor.Role == domain.RoleFarmer && actor.FarmID != batch.FarmID {
		return domain.SubstrateBatch{}, domain.ErrForbidden
	}
	return batch, nil
}
func (s *BatchService) List(ctx context.Context, filter repository.BatchFilter) (pagination.Result[domain.SubstrateBatch], error) {
	actor, err := IdentityFrom(ctx)
	if err != nil {
		return pagination.Result[domain.SubstrateBatch]{}, err
	}
	filter.TenantID = actor.TenantID
	if actor.Role == domain.RoleFarmer {
		filter.FarmID = actor.FarmID
	}
	items, total, err := s.repo.ListBatches(ctx, filter)
	if err != nil {
		return pagination.Result[domain.SubstrateBatch]{}, err
	}
	return pagination.Build(items, total, filter.Page), nil
}

func newAudit(actor Identity, requestID, action, object, objectID string, outcome audit.Outcome, metadata any, now time.Time) (audit.Event, error) {
	payload, err := json.Marshal(metadata)
	if err != nil {
		return audit.Event{}, err
	}
	id, err := auth.NewID("aud")
	if err != nil {
		return audit.Event{}, err
	}
	return audit.Event{ID: id, TenantID: actor.TenantID, ActorID: actor.UserID, RequestID: requestID, Action: action, Object: object, ObjectID: objectID, Outcome: outcome, Metadata: payload, CreatedAt: now}, nil
}
