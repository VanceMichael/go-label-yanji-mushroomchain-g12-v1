package domain

import "time"

type User struct {
	ID           string    `json:"id"`
	TenantID     string    `json:"tenant_id"`
	FarmID       string    `json:"farm_id,omitempty"`
	Email        string    `json:"email"`
	DisplayName  string    `json:"display_name"`
	Role         Role      `json:"role"`
	PasswordHash string    `json:"-"`
	Active       bool      `json:"active"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type Session struct {
	ID        string     `json:"id"`
	UserID    string     `json:"user_id"`
	TokenHash string     `json:"-"`
	ExpiresAt time.Time  `json:"expires_at"`
	RevokedAt *time.Time `json:"revoked_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

func (s Session) ActiveAt(now time.Time) bool {
	return s.RevokedAt == nil && now.Before(s.ExpiresAt)
}

type Farm struct {
	ID             string    `json:"id"`
	TenantID       string    `json:"tenant_id"`
	Name           string    `json:"name"`
	Village        string    `json:"village"`
	OwnerUserID    string    `json:"owner_user_id"`
	SettlementName string    `json:"settlement_name"`
	CreatedAt      time.Time `json:"created_at"`
}

type SubstrateBatch struct {
	ID                string      `json:"id"`
	TenantID          string      `json:"tenant_id"`
	FarmID            string      `json:"farm_id"`
	Code              string      `json:"code"`
	Species           string      `json:"species"`
	ProducedAt        time.Time   `json:"produced_at"`
	ExpiresAt         time.Time   `json:"expires_at"`
	QuantityProduced  int64       `json:"quantity_produced"`
	QuantityAvailable int64       `json:"quantity_available"`
	UnitPriceCents    int64       `json:"unit_price_cents"`
	Status            BatchStatus `json:"status"`
	Version           int64       `json:"version"`
	CreatedAt         time.Time   `json:"created_at"`
	UpdatedAt         time.Time   `json:"updated_at"`
}

func (b SubstrateBatch) Validate() error {
	if b.ID == "" {
		return FieldError{Field: "id", Message: "required"}
	}
	if b.TenantID == "" {
		return FieldError{Field: "tenant_id", Message: "required"}
	}
	if b.FarmID == "" {
		return FieldError{Field: "farm_id", Message: "required"}
	}
	if b.Code == "" {
		return FieldError{Field: "code", Message: "required"}
	}
	if b.QuantityProduced <= 0 {
		return FieldError{Field: "quantity_produced", Message: "must be positive"}
	}
	if b.QuantityAvailable < 0 || b.QuantityAvailable > b.QuantityProduced {
		return FieldError{Field: "quantity_available", Message: "outside produced quantity"}
	}
	if b.UnitPriceCents <= 0 {
		return FieldError{Field: "unit_price_cents", Message: "must be positive"}
	}
	if !b.ExpiresAt.After(b.ProducedAt) {
		return FieldError{Field: "expires_at", Message: "must follow produced_at"}
	}
	return nil
}

type QualityInspection struct {
	ID          string             `json:"id"`
	TenantID    string             `json:"tenant_id"`
	BatchID     string             `json:"batch_id"`
	InspectorID string             `json:"inspector_id"`
	Decision    InspectionDecision `json:"decision"`
	MoistureBP  int                `json:"moisture_bp"`
	SampleCount int                `json:"sample_count"`
	Notes       string             `json:"notes"`
	InspectedAt time.Time          `json:"inspected_at"`
	CreatedAt   time.Time          `json:"created_at"`
}

type Order struct {
	ID                 string      `json:"id"`
	TenantID           string      `json:"tenant_id"`
	BuyerName          string      `json:"buyer_name"`
	DeliveryRegion     string      `json:"delivery_region"`
	Status             OrderStatus `json:"status"`
	IdempotencyKey     string      `json:"-"`
	RequestFingerprint string      `json:"-"`
	Version            int64       `json:"version"`
	TotalCents         int64       `json:"total_cents"`
	RequestedAt        time.Time   `json:"requested_at"`
	DueAt              time.Time   `json:"due_at"`
	CreatedAt          time.Time   `json:"created_at"`
	UpdatedAt          time.Time   `json:"updated_at"`
	Lines              []OrderLine `json:"lines,omitempty"`
}

type OrderLine struct {
	ID             string `json:"id"`
	OrderID        string `json:"order_id"`
	Species        string `json:"species"`
	Quantity       int64  `json:"quantity"`
	UnitPriceCents int64  `json:"unit_price_cents"`
}

func (o Order) Validate() error {
	if o.ID == "" || o.TenantID == "" {
		return FieldError{Field: "order", Message: "id and tenant are required"}
	}
	if o.BuyerName == "" || o.DeliveryRegion == "" {
		return FieldError{Field: "buyer", Message: "name and delivery region are required"}
	}
	if len(o.Lines) == 0 {
		return FieldError{Field: "lines", Message: "at least one line is required"}
	}
	if !o.DueAt.After(o.RequestedAt) {
		return FieldError{Field: "due_at", Message: "must follow requested_at"}
	}
	for _, line := range o.Lines {
		if line.Quantity <= 0 || line.UnitPriceCents <= 0 || line.Species == "" {
			return FieldError{Field: "lines", Message: "invalid order line"}
		}
	}
	return nil
}

type InventoryAllocation struct {
	ID        string    `json:"id"`
	TenantID  string    `json:"tenant_id"`
	OrderID   string    `json:"order_id"`
	LineID    string    `json:"line_id"`
	BatchID   string    `json:"batch_id"`
	Quantity  int64     `json:"quantity"`
	CreatedAt time.Time `json:"created_at"`
}

type Shipment struct {
	ID             string         `json:"id"`
	TenantID       string         `json:"tenant_id"`
	OrderID        string         `json:"order_id"`
	Status         ShipmentStatus `json:"status"`
	Carrier        string         `json:"carrier"`
	TrackingNumber string         `json:"tracking_number"`
	ClaimedBy      string         `json:"claimed_by,omitempty"`
	LeaseUntil     *time.Time     `json:"lease_until,omitempty"`
	Attempts       int            `json:"attempts"`
	LastError      string         `json:"last_error,omitempty"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
}

type Settlement struct {
	ID          string           `json:"id"`
	TenantID    string           `json:"tenant_id"`
	FarmID      string           `json:"farm_id"`
	OrderID     string           `json:"order_id"`
	AmountCents int64            `json:"amount_cents"`
	Status      SettlementStatus `json:"status"`
	Version     int64            `json:"version"`
	ApprovedBy  string           `json:"approved_by,omitempty"`
	ApprovedAt  *time.Time       `json:"approved_at,omitempty"`
	PaidAt      *time.Time       `json:"paid_at,omitempty"`
	CreatedAt   time.Time        `json:"created_at"`
	UpdatedAt   time.Time        `json:"updated_at"`
}

type OutboxEvent struct {
	ID            string       `json:"id"`
	TenantID      string       `json:"tenant_id"`
	AggregateType string       `json:"aggregate_type"`
	AggregateID   string       `json:"aggregate_id"`
	EventType     string       `json:"event_type"`
	Payload       []byte       `json:"payload"`
	Status        OutboxStatus `json:"status"`
	Attempts      int          `json:"attempts"`
	AvailableAt   time.Time    `json:"available_at"`
	LeaseOwner    string       `json:"lease_owner,omitempty"`
	LeaseUntil    *time.Time   `json:"lease_until,omitempty"`
	LastError     string       `json:"last_error,omitempty"`
	CreatedAt     time.Time    `json:"created_at"`
	DeliveredAt   *time.Time   `json:"delivered_at,omitempty"`
}
