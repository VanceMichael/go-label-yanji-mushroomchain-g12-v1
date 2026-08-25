package sqlite

import (
	"context"
	"fmt"
	"time"

	"github.com/VanceMichael/go-label-yanji-mushroomchain-g12-v1/internal/domain"
)

func (s *Store) CreateTenant(ctx context.Context, id, name, timezone string, at time.Time) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO tenants(id,name,timezone,created_at) VALUES(?,?,?,?)`, id, name, timezone, formatTime(at))
	return mapSQLError(err)
}

func (s *Store) TenantExists(ctx context.Context, id string) (bool, error) {
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM tenants WHERE id=?`, id).Scan(&count); err != nil {
		return false, mapSQLError(err)
	}
	return count == 1, nil
}

func (s *Store) SeedTenant(ctx context.Context, tenantID, name string, users []domain.User) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tenant seed: %w", err)
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `INSERT INTO tenants(id,name,timezone,created_at) VALUES(?,?,?,?)`, tenantID, name, "Asia/Shanghai", formatTime(time.Now().UTC())); err != nil {
		return mapSQLError(err)
	}
	nested := &txStore{q: tx}
	for _, user := range users {
		if err = nested.CreateUser(ctx, user); err != nil {
			return err
		}
	}
	return tx.Commit()
}
