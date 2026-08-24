package testkit

import (
	"context"
	"testing"
	"time"

	"github.com/VanceMichael/go-label-yanji-mushroomchain-g12-v1/internal/auth"
	"github.com/VanceMichael/go-label-yanji-mushroomchain-g12-v1/internal/domain"
	"github.com/VanceMichael/go-label-yanji-mushroomchain-g12-v1/internal/storage/sqlite"
)

type Fixture struct {
	Store    *sqlite.Store
	Now      time.Time
	TenantID string
	Password string
	Users    map[domain.Role]domain.User
	Farm     domain.Farm
}

func New(t *testing.T) Fixture {
	t.Helper()
	ctx := context.Background()
	store, err := sqlite.Open(ctx, t.TempDir()+"/fixture.db")
	if err != nil {
		t.Fatalf("open fixture database: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err = store.Migrate(ctx); err != nil {
		t.Fatalf("migrate fixture database: %v", err)
	}
	now := time.Date(2026, 8, 25, 8, 0, 0, 0, time.UTC)
	password := "correct-horse-battery-staple"
	hash, err := auth.HashPassword(password)
	if err != nil {
		t.Fatalf("hash fixture password: %v", err)
	}
	roles := []domain.Role{domain.RoleOperator, domain.RoleFarmer, domain.RoleInspector, domain.RoleDispatcher, domain.RoleFinance}
	users := make(map[domain.Role]domain.User, len(roles))
	for index, role := range roles {
		users[role] = domain.User{ID: "user-" + string(role), TenantID: "tenant-xiji", Email: string(role) + "@test.local", DisplayName: "Test " + string(role), Role: role, PasswordHash: hash, Active: true, CreatedAt: now.Add(time.Duration(index) * time.Second), UpdatedAt: now.Add(time.Duration(index) * time.Second)}
	}
	list := make([]domain.User, 0, len(users))
	for _, role := range roles {
		list = append(list, users[role])
	}
	if err = store.SeedTenant(ctx, "tenant-xiji", "Xiji Test Cooperative", list); err != nil {
		t.Fatalf("seed fixture tenant: %v", err)
	}
	farm := domain.Farm{ID: "farm-one", TenantID: "tenant-xiji", Name: "Jiangtaipu Farm", Village: "Jiangtaipu", OwnerUserID: users[domain.RoleFarmer].ID, SettlementName: "Farmer One", CreatedAt: now}
	if err = store.CreateFarm(ctx, farm); err != nil {
		t.Fatalf("create fixture farm: %v", err)
	}
	return Fixture{Store: store, Now: now, TenantID: "tenant-xiji", Password: password, Users: users, Farm: farm}
}

func (f Fixture) Identity(role domain.Role) context.Context {
	user := f.Users[role]
	return contextWithIdentity(context.Background(), user, f.Farm.ID)
}

// Defined locally to keep this fixture independent of service package cycles.
func contextWithIdentity(ctx context.Context, user domain.User, farmID string) context.Context {
	return context.WithValue(ctx, fixtureIdentityKey{}, struct {
		User   domain.User
		FarmID string
	}{User: user, FarmID: farmID})
}

type fixtureIdentityKey struct{}

func (f Fixture) Batch(status domain.BatchStatus, quantity int64) domain.SubstrateBatch {
	return domain.SubstrateBatch{ID: "batch-one", TenantID: f.TenantID, FarmID: f.Farm.ID, Code: "XB-20260825-001", Species: "oyster", ProducedAt: f.Now.Add(-24 * time.Hour), ExpiresAt: f.Now.Add(60 * 24 * time.Hour), QuantityProduced: quantity, QuantityAvailable: quantity, UnitPriceCents: 350, Status: status, Version: 1, CreatedAt: f.Now, UpdatedAt: f.Now}
}

func (f Fixture) Order(status domain.OrderStatus, quantity int64) domain.Order {
	return domain.Order{ID: "order-one", TenantID: f.TenantID, BuyerName: "Minning Distributor", DeliveryRegion: "Yinchuan", Status: status, IdempotencyKey: "fixture-order-one", Version: 1, TotalCents: quantity * 400, RequestedAt: f.Now, DueAt: f.Now.Add(72 * time.Hour), CreatedAt: f.Now, UpdatedAt: f.Now, Lines: []domain.OrderLine{{ID: "line-one", OrderID: "order-one", Species: "oyster", Quantity: quantity, UnitPriceCents: 400}}}
}
