package domain

type BatchStatus string

const (
	BatchRegistered BatchStatus = "registered"
	BatchSampling   BatchStatus = "sampling"
	BatchReleased   BatchStatus = "released"
	BatchRejected   BatchStatus = "rejected"
	BatchExhausted  BatchStatus = "exhausted"
	BatchArchived   BatchStatus = "archived"
)

func (s BatchStatus) CanTransition(to BatchStatus) bool {
	allowed := map[BatchStatus]map[BatchStatus]bool{
		BatchRegistered: {BatchSampling: true},
		BatchSampling:   {BatchReleased: true, BatchRejected: true},
		BatchReleased:   {BatchExhausted: true},
		BatchRejected:   {BatchArchived: true},
		BatchExhausted:  {BatchArchived: true},
	}
	return allowed[s][to]
}

type InspectionDecision string

const (
	InspectionPending  InspectionDecision = "pending"
	InspectionApproved InspectionDecision = "approved"
	InspectionRejected InspectionDecision = "rejected"
)

type OrderStatus string

const (
	OrderDraft     OrderStatus = "draft"
	OrderConfirmed OrderStatus = "confirmed"
	OrderAllocated OrderStatus = "allocated"
	OrderInTransit OrderStatus = "in_transit"
	OrderDelivered OrderStatus = "delivered"
	OrderCancelled OrderStatus = "cancelled"
	OrderSettled   OrderStatus = "settled"
)

func (s OrderStatus) CanTransition(to OrderStatus) bool {
	allowed := map[OrderStatus]map[OrderStatus]bool{
		OrderDraft:     {OrderConfirmed: true, OrderCancelled: true},
		OrderConfirmed: {OrderAllocated: true, OrderCancelled: true},
		OrderAllocated: {OrderInTransit: true, OrderCancelled: true},
		OrderInTransit: {OrderDelivered: true},
		OrderDelivered: {OrderSettled: true},
	}
	return allowed[s][to]
}

type ShipmentStatus string

const (
	ShipmentPending    ShipmentStatus = "pending"
	ShipmentClaimed    ShipmentStatus = "claimed"
	ShipmentDispatched ShipmentStatus = "dispatched"
	ShipmentDelivered  ShipmentStatus = "delivered"
	ShipmentFailed     ShipmentStatus = "failed"
)

type SettlementStatus string

const (
	SettlementPending  SettlementStatus = "pending"
	SettlementApproved SettlementStatus = "approved"
	SettlementPaid     SettlementStatus = "paid"
	SettlementFailed   SettlementStatus = "failed"
)

type OutboxStatus string

const (
	OutboxPending   OutboxStatus = "pending"
	OutboxClaimed   OutboxStatus = "claimed"
	OutboxDelivered OutboxStatus = "delivered"
	OutboxDead      OutboxStatus = "dead"
)

type Role string

const (
	RoleOperator   Role = "operator"
	RoleFarmer     Role = "farmer"
	RoleInspector  Role = "inspector"
	RoleDispatcher Role = "dispatcher"
	RoleFinance    Role = "finance"
)

func (r Role) Valid() bool {
	switch r {
	case RoleOperator, RoleFarmer, RoleInspector, RoleDispatcher, RoleFinance:
		return true
	default:
		return false
	}
}
