package repository

import (
	"context"
	"time"

	"github.com/VanceMichael/go-label-yanji-mushroomchain-g12-v1/internal/audit"
	"github.com/VanceMichael/go-label-yanji-mushroomchain-g12-v1/internal/domain"
	"github.com/VanceMichael/go-label-yanji-mushroomchain-g12-v1/internal/pagination"
)

type Transactor interface {
	WithinTx(context.Context, func(Tx) error) error
}

type Tx interface {
	Users
	Sessions
	Farms
	Batches
	Orders
	Shipments
	Settlements
	Outbox
	Audits
}

type Users interface {
	CreateUser(context.Context, domain.User) error
	FindUserByEmail(context.Context, string, string) (domain.User, error)
	GetUser(context.Context, string, string) (domain.User, error)
}

type Sessions interface {
	CreateSession(context.Context, domain.Session) error
	FindSessionByTokenHash(context.Context, string) (domain.Session, domain.User, error)
	RevokeUserSessions(context.Context, string, time.Time) error
	DeleteExpiredSessions(context.Context, time.Time) (int64, error)
}

type Farms interface {
	CreateFarm(context.Context, domain.Farm) error
	GetFarm(context.Context, string, string) (domain.Farm, error)
}

type BatchFilter struct {
	TenantID string
	FarmID   string
	Species  string
	Status   domain.BatchStatus
	Page     pagination.Page
}

type Batches interface {
	CreateBatch(context.Context, domain.SubstrateBatch) error
	GetBatch(context.Context, string, string) (domain.SubstrateBatch, error)
	ListBatches(context.Context, BatchFilter) ([]domain.SubstrateBatch, int, error)
	TransitionBatch(context.Context, string, string, int64, domain.BatchStatus, time.Time) error
	CreateInspection(context.Context, domain.QualityInspection) error
	ListInspections(context.Context, string, string) ([]domain.QualityInspection, error)
	AllocateBatch(context.Context, string, string, int64, int64, time.Time) error
	RestoreBatch(context.Context, string, string, int64, time.Time) error
}

type OrderFilter struct {
	TenantID  string
	Status    domain.OrderStatus
	DueBefore *time.Time
	Page      pagination.Page
}

type Orders interface {
	CreateOrder(context.Context, domain.Order) error
	GetOrder(context.Context, string, string) (domain.Order, error)
	FindOrderByIdempotency(context.Context, string, string) (domain.Order, error)
	ListOrders(context.Context, OrderFilter) ([]domain.Order, int, error)
	TransitionOrder(context.Context, string, string, int64, domain.OrderStatus, time.Time) error
	CreateAllocation(context.Context, domain.InventoryAllocation) error
	ListAllocations(context.Context, string, string) ([]domain.InventoryAllocation, error)
}

type Shipments interface {
	CreateShipment(context.Context, domain.Shipment) error
	GetShipmentByOrder(context.Context, string, string) (domain.Shipment, error)
	ClaimShipment(context.Context, string, time.Time, time.Time) (domain.Shipment, error)
	CompleteShipment(context.Context, string, string, domain.ShipmentStatus, string, time.Time) error
}

type Settlements interface {
	CreateSettlement(context.Context, domain.Settlement) error
	GetSettlement(context.Context, string, string) (domain.Settlement, error)
	ListSettlementsByOrder(context.Context, string, string) ([]domain.Settlement, error)
	ApproveSettlement(context.Context, string, string, int64, string, time.Time) error
	MarkSettlementPaid(context.Context, string, string, int64, time.Time) error
}

type Outbox interface {
	CreateOutboxEvent(context.Context, domain.OutboxEvent) error
	ClaimOutbox(context.Context, string, time.Time, time.Time) (domain.OutboxEvent, error)
	CompleteOutbox(context.Context, string, string, time.Time) error
	FailOutbox(context.Context, string, string, string, int, time.Time) error
}

type Audits interface {
	CreateAudit(context.Context, audit.Event) error
	ListAudit(context.Context, string, string, string, pagination.Page) ([]audit.Event, int, error)
}
