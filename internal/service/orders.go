package service

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/VanceMichael/go-label-yanji-mushroomchain-g12-v1/internal/audit"
	"github.com/VanceMichael/go-label-yanji-mushroomchain-g12-v1/internal/auth"
	"github.com/VanceMichael/go-label-yanji-mushroomchain-g12-v1/internal/clock"
	"github.com/VanceMichael/go-label-yanji-mushroomchain-g12-v1/internal/domain"
	"github.com/VanceMichael/go-label-yanji-mushroomchain-g12-v1/internal/pagination"
	"github.com/VanceMichael/go-label-yanji-mushroomchain-g12-v1/internal/repository"
)

type OrderService struct {
	tx    repository.Transactor
	repo  repository.Tx
	clock clock.Clock
}

func NewOrders(tx repository.Transactor, repo repository.Tx, c clock.Clock) *OrderService {
	return &OrderService{tx: tx, repo: repo, clock: c}
}

type CreateOrderInput struct {
	BuyerName, DeliveryRegion, IdempotencyKey, RequestID string
	DueAt                                                time.Time
	Lines                                                []domain.OrderLine
}

func (s *OrderService) Create(ctx context.Context, input CreateOrderInput) (domain.Order, error) {
	actor, err := RequireRoles(ctx, domain.RoleOperator, domain.RoleDispatcher)
	if err != nil {
		return domain.Order{}, err
	}
	if existing, e := s.repo.FindOrderByIdempotency(ctx, actor.TenantID, input.IdempotencyKey); e == nil {
		return existing, nil
	} else if !errors.Is(e, domain.ErrNotFound) {
		return domain.Order{}, e
	}
	now := s.clock.Now()
	id, err := auth.NewID("ord")
	if err != nil {
		return domain.Order{}, err
	}
	order := domain.Order{ID: id, TenantID: actor.TenantID, BuyerName: input.BuyerName, DeliveryRegion: input.DeliveryRegion, Status: domain.OrderDraft, IdempotencyKey: input.IdempotencyKey, Version: 1, RequestedAt: now, DueAt: input.DueAt.UTC(), CreatedAt: now, UpdatedAt: now}
	for index, line := range input.Lines {
		line.ID, err = auth.NewID("orl")
		if err != nil {
			return domain.Order{}, err
		}
		line.OrderID = id
		order.TotalCents += line.Quantity * line.UnitPriceCents
		order.Lines = append(order.Lines, line)
		_ = index
	}
	if err = order.Validate(); err != nil {
		return domain.Order{}, err
	}
	err = s.tx.WithinTx(ctx, func(tx repository.Tx) error {
		if e := tx.CreateOrder(ctx, order); e != nil {
			return e
		}
		if e := tx.TransitionOrder(ctx, actor.TenantID, id, 1, domain.OrderConfirmed, now); e != nil {
			return e
		}
		event, e := newAudit(actor, input.RequestID, "order.create", "order", id, audit.OutcomeSucceeded, map[string]any{"lines": len(order.Lines), "total_cents": order.TotalCents}, now)
		if e != nil {
			return e
		}
		return tx.CreateAudit(ctx, event)
	})
	if err != nil {
		if errors.Is(err, domain.ErrConflict) {
			return s.repo.FindOrderByIdempotency(ctx, actor.TenantID, input.IdempotencyKey)
		}
		return domain.Order{}, err
	}
	return s.repo.GetOrder(ctx, actor.TenantID, id)
}

type AllocateInput struct {
	OrderID, RequestID string
	ExpectedVersion    int64
}

func (s *OrderService) Allocate(ctx context.Context, input AllocateInput) (domain.Order, error) {
	actor, err := RequireRoles(ctx, domain.RoleDispatcher)
	if err != nil {
		return domain.Order{}, err
	}
	now := s.clock.Now()
	order, err := s.repo.GetOrder(ctx, actor.TenantID, input.OrderID)
	if err != nil {
		return domain.Order{}, err
	}
	if order.Version != input.ExpectedVersion {
		return domain.Order{}, domain.ErrVersionConflict
	}
	if order.Status != domain.OrderConfirmed {
		return domain.Order{}, domain.StateError{Entity: "order", From: string(order.Status), To: string(domain.OrderAllocated)}
	}

	reservations := make([]repository.BatchAllocation, 0)
	reservationIndex := make(map[string]int)
	plannedQuantity := make(map[string]int64)
	allocations := make([]domain.InventoryAllocation, 0)
	for _, line := range order.Lines {
		candidates, _, e := s.repo.ListBatches(ctx, repository.BatchFilter{TenantID: actor.TenantID, Species: line.Species, Status: domain.BatchReleased, Page: pagination.Page{Number: 1, Size: 100}})
		if e != nil {
			return domain.Order{}, e
		}
		remaining := line.Quantity
		for _, batch := range candidates {
			available := batch.QuantityAvailable - plannedQuantity[batch.ID]
			if !batch.ExpiresAt.After(now) || available <= 0 || remaining == 0 {
				continue
			}
			quantity := remaining
			if available < quantity {
				quantity = available
			}
			allocationID, e := auth.NewID("alc")
			if e != nil {
				return domain.Order{}, e
			}
			allocations = append(allocations, domain.InventoryAllocation{ID: allocationID, TenantID: actor.TenantID, OrderID: order.ID, LineID: line.ID, BatchID: batch.ID, Quantity: quantity, CreatedAt: now})
			plannedQuantity[batch.ID] += quantity
			if index, ok := reservationIndex[batch.ID]; ok {
				reservations[index].Quantity += quantity
			} else {
				reservationIndex[batch.ID] = len(reservations)
				reservations = append(reservations, repository.BatchAllocation{BatchID: batch.ID, Quantity: quantity, Version: batch.Version})
			}
			remaining -= quantity
		}
		if remaining > 0 {
			return domain.Order{}, domain.ErrCapacity
		}
	}
	err = s.tx.WithinTx(ctx, func(tx repository.Tx) error {
		current, e := tx.GetOrder(ctx, actor.TenantID, input.OrderID)
		if e != nil {
			return e
		}
		if current.Version != input.ExpectedVersion {
			return domain.ErrVersionConflict
		}
		if current.Status != domain.OrderConfirmed {
			return domain.StateError{Entity: "order", From: string(current.Status), To: string(domain.OrderAllocated)}
		}
		if e = tx.AllocateBatches(ctx, actor.TenantID, reservations, now); e != nil {
			return e
		}
		if e = tx.CreateAllocations(ctx, allocations); e != nil {
			return e
		}
		if e := tx.TransitionOrder(ctx, actor.TenantID, current.ID, current.Version, domain.OrderAllocated, now); e != nil {
			return e
		}
		shipmentID, e := auth.NewID("shp")
		if e != nil {
			return e
		}
		if e = tx.CreateShipment(ctx, domain.Shipment{ID: shipmentID, TenantID: actor.TenantID, OrderID: current.ID, Status: domain.ShipmentPending, CreatedAt: now, UpdatedAt: now}); e != nil {
			return e
		}
		event, e := newAudit(actor, input.RequestID, "order.allocate", "order", current.ID, audit.OutcomeSucceeded, map[string]any{"line_count": len(current.Lines)}, now)
		if e != nil {
			return e
		}
		return tx.CreateAudit(ctx, event)
	})
	if err != nil {
		return domain.Order{}, err
	}
	return s.repo.GetOrder(ctx, actor.TenantID, input.OrderID)
}

func (s *OrderService) Get(ctx context.Context, id string) (domain.Order, error) {
	actor, err := IdentityFrom(ctx)
	if err != nil {
		return domain.Order{}, err
	}
	return s.repo.GetOrder(ctx, actor.TenantID, id)
}
func (s *OrderService) List(ctx context.Context, f repository.OrderFilter) (pagination.Result[domain.Order], error) {
	actor, err := IdentityFrom(ctx)
	if err != nil {
		return pagination.Result[domain.Order]{}, err
	}
	f.TenantID = actor.TenantID
	items, total, err := s.repo.ListOrders(ctx, f)
	if err != nil {
		return pagination.Result[domain.Order]{}, err
	}
	return pagination.Build(items, total, f.Page), nil
}

type DeliveryInput struct {
	OrderID, RequestID string
	ExpectedVersion    int64
}

func (s *OrderService) MarkDelivered(ctx context.Context, input DeliveryInput) (domain.Order, error) {
	actor, err := RequireRoles(ctx, domain.RoleDispatcher)
	if err != nil {
		return domain.Order{}, err
	}
	now := s.clock.Now()
	err = s.tx.WithinTx(ctx, func(tx repository.Tx) error {
		order, e := tx.GetOrder(ctx, actor.TenantID, input.OrderID)
		if e != nil {
			return e
		}
		if order.Version != input.ExpectedVersion {
			return domain.ErrVersionConflict
		}
		if order.Status == domain.OrderAllocated {
			if e = tx.TransitionOrder(ctx, actor.TenantID, order.ID, order.Version, domain.OrderInTransit, now); e != nil {
				return e
			}
			order.Version++
			order.Status = domain.OrderInTransit
		}
		if e = tx.TransitionOrder(ctx, actor.TenantID, order.ID, order.Version, domain.OrderDelivered, now); e != nil {
			return e
		}
		allocations, e := tx.ListAllocations(ctx, actor.TenantID, order.ID)
		if e != nil {
			return e
		}
		byFarm := map[string]int64{}
		for _, allocation := range allocations {
			batch, e := tx.GetBatch(ctx, actor.TenantID, allocation.BatchID)
			if e != nil {
				return e
			}
			byFarm[batch.FarmID] += allocation.Quantity * batch.UnitPriceCents
		}
		for farmID, amount := range byFarm {
			settlementID, e := auth.NewID("set")
			if e != nil {
				return e
			}
			if e = tx.CreateSettlement(ctx, domain.Settlement{ID: settlementID, TenantID: actor.TenantID, FarmID: farmID, OrderID: order.ID, AmountCents: amount, Status: domain.SettlementPending, Version: 1, CreatedAt: now, UpdatedAt: now}); e != nil {
				return e
			}
			payload, _ := json.Marshal(map[string]any{"settlement_id": settlementID, "farm_id": farmID, "amount_cents": amount})
			outboxID, e := auth.NewID("evt")
			if e != nil {
				return e
			}
			if e = tx.CreateOutboxEvent(ctx, domain.OutboxEvent{ID: outboxID, TenantID: actor.TenantID, AggregateType: "settlement", AggregateID: settlementID, EventType: "settlement.created", Payload: payload, Status: domain.OutboxPending, AvailableAt: now, CreatedAt: now}); e != nil {
				return e
			}
		}
		event, e := newAudit(actor, input.RequestID, "order.deliver", "order", order.ID, audit.OutcomeSucceeded, map[string]any{"settlements": len(byFarm)}, now)
		if e != nil {
			return e
		}
		return tx.CreateAudit(ctx, event)
	})
	if err != nil {
		return domain.Order{}, err
	}
	return s.repo.GetOrder(ctx, actor.TenantID, input.OrderID)
}
