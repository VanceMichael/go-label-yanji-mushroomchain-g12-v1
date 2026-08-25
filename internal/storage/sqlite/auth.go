package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/VanceMichael/go-label-yanji-mushroomchain-g12-v1/internal/domain"
)

func createUser(ctx context.Context, q queryer, user domain.User) error {
	_, err := q.ExecContext(ctx, `
		INSERT INTO users(id,tenant_id,farm_id,email,display_name,role,password_hash,active,created_at,updated_at)
		VALUES(?,?,?,?,?,?,?,?,?,?)`, user.ID, user.TenantID, nullEmpty(user.FarmID), user.Email,
		user.DisplayName, user.Role, user.PasswordHash, boolInt(user.Active), formatTime(user.CreatedAt), formatTime(user.UpdatedAt))
	return mapSQLError(err)
}

func (s *Store) CreateUser(ctx context.Context, user domain.User) error {
	return createUser(ctx, s.q(), user)
}
func (s *txStore) CreateUser(ctx context.Context, user domain.User) error {
	return createUser(ctx, s.qer(), user)
}

func scanUser(row interface{ Scan(...any) error }) (domain.User, error) {
	var user domain.User
	var farm sql.NullString
	var active int
	var created, updated string
	err := row.Scan(&user.ID, &user.TenantID, &farm, &user.Email, &user.DisplayName, &user.Role,
		&user.PasswordHash, &active, &created, &updated)
	if err != nil {
		return domain.User{}, mapSQLError(err)
	}
	user.FarmID = farm.String
	user.Active = active == 1
	user.CreatedAt, err = parseTime(created)
	if err != nil {
		return domain.User{}, err
	}
	user.UpdatedAt, err = parseTime(updated)
	return user, err
}

const userColumns = `id,tenant_id,farm_id,email,display_name,role,password_hash,active,created_at,updated_at`

func findUserByEmail(ctx context.Context, q queryer, tenantID, email string) (domain.User, error) {
	return scanUser(q.QueryRowContext(ctx, `SELECT `+userColumns+` FROM users WHERE tenant_id=? AND lower(email)=lower(?)`, tenantID, email))
}

func (s *Store) FindUserByEmail(ctx context.Context, tenantID, email string) (domain.User, error) {
	return findUserByEmail(ctx, s.q(), tenantID, email)
}
func (s *txStore) FindUserByEmail(ctx context.Context, tenantID, email string) (domain.User, error) {
	return findUserByEmail(ctx, s.qer(), tenantID, email)
}

func getUser(ctx context.Context, q queryer, tenantID, id string) (domain.User, error) {
	return scanUser(q.QueryRowContext(ctx, `SELECT `+userColumns+` FROM users WHERE tenant_id=? AND id=?`, tenantID, id))
}

func (s *Store) GetUser(ctx context.Context, tenantID, id string) (domain.User, error) {
	return getUser(ctx, s.q(), tenantID, id)
}
func (s *txStore) GetUser(ctx context.Context, tenantID, id string) (domain.User, error) {
	return getUser(ctx, s.qer(), tenantID, id)
}

func createSession(ctx context.Context, q queryer, session domain.Session) error {
	_, err := q.ExecContext(ctx, `INSERT INTO sessions(id,user_id,token_hash,expires_at,revoked_at,created_at) VALUES(?,?,?,?,?,?)`,
		session.ID, session.UserID, session.TokenHash, formatTime(session.ExpiresAt), nullableTimeValue(session.RevokedAt), formatTime(session.CreatedAt))
	return mapSQLError(err)
}

func (s *Store) CreateSession(ctx context.Context, session domain.Session) error {
	return createSession(ctx, s.q(), session)
}
func (s *txStore) CreateSession(ctx context.Context, session domain.Session) error {
	return createSession(ctx, s.qer(), session)
}

func findSession(ctx context.Context, q queryer, tokenHash string) (domain.Session, domain.User, error) {
	row := q.QueryRowContext(ctx, `SELECT s.id,s.user_id,s.token_hash,s.expires_at,s.revoked_at,s.created_at,
		u.id,u.tenant_id,u.farm_id,u.email,u.display_name,u.role,u.password_hash,u.active,u.created_at,u.updated_at
		FROM sessions s JOIN users u ON u.id=s.user_id WHERE s.token_hash=?`, tokenHash)
	var session domain.Session
	var user domain.User
	var expires, sessionCreated, userCreated, userUpdated string
	var revoked, farm sql.NullString
	var active int
	err := row.Scan(&session.ID, &session.UserID, &session.TokenHash, &expires, &revoked, &sessionCreated,
		&user.ID, &user.TenantID, &farm, &user.Email, &user.DisplayName, &user.Role, &user.PasswordHash,
		&active, &userCreated, &userUpdated)
	if err != nil {
		mapped := mapSQLError(err)
		if mapped != nil && !errors.Is(mapped, domain.ErrNotFound) {
			return domain.Session{}, domain.User{}, domain.DependencyError{Operation: "find session", Err: mapped}
		}
		return domain.Session{}, domain.User{}, mapped
	}
	session.ExpiresAt, err = parseTime(expires)
	if err != nil {
		return domain.Session{}, domain.User{}, err
	}
	session.RevokedAt, err = nullableTime(revoked)
	if err != nil {
		return domain.Session{}, domain.User{}, err
	}
	session.CreatedAt, err = parseTime(sessionCreated)
	if err != nil {
		return domain.Session{}, domain.User{}, err
	}
	user.FarmID, user.Active = farm.String, active == 1
	user.CreatedAt, err = parseTime(userCreated)
	if err != nil {
		return domain.Session{}, domain.User{}, err
	}
	user.UpdatedAt, err = parseTime(userUpdated)
	return session, user, err
}

func (s *Store) FindSessionByTokenHash(ctx context.Context, hash string) (domain.Session, domain.User, error) {
	return findSession(ctx, s.q(), hash)
}
func (s *txStore) FindSessionByTokenHash(ctx context.Context, hash string) (domain.Session, domain.User, error) {
	return findSession(ctx, s.qer(), hash)
}

func revokeSession(ctx context.Context, q queryer, userID, sessionID string, at time.Time) error {
	result, err := q.ExecContext(ctx, `UPDATE sessions SET revoked_at=? WHERE id=? AND user_id=? AND revoked_at IS NULL`, formatTime(at), sessionID, userID)
	if err != nil {
		return mapSQLError(err)
	}
	return requireAffected(result, "session")
}

func (s *Store) RevokeSession(ctx context.Context, userID, sessionID string, at time.Time) error {
	return revokeSession(ctx, s.q(), userID, sessionID, at)
}
func (s *txStore) RevokeSession(ctx context.Context, userID, sessionID string, at time.Time) error {
	return revokeSession(ctx, s.qer(), userID, sessionID, at)
}

func deleteExpired(ctx context.Context, q queryer, at time.Time) (int64, error) {
	result, err := q.ExecContext(ctx, `DELETE FROM sessions WHERE expires_at<=? OR revoked_at IS NOT NULL`, formatTime(at))
	if err != nil {
		return 0, mapSQLError(err)
	}
	return result.RowsAffected()
}

func (s *Store) DeleteExpiredSessions(ctx context.Context, at time.Time) (int64, error) {
	return deleteExpired(ctx, s.q(), at)
}
func (s *txStore) DeleteExpiredSessions(ctx context.Context, at time.Time) (int64, error) {
	return deleteExpired(ctx, s.qer(), at)
}

func createFarm(ctx context.Context, q queryer, farm domain.Farm) error {
	_, err := q.ExecContext(ctx, `INSERT INTO farms(id,tenant_id,name,village,owner_user_id,settlement_name,created_at) VALUES(?,?,?,?,?,?,?)`,
		farm.ID, farm.TenantID, farm.Name, farm.Village, farm.OwnerUserID, farm.SettlementName, formatTime(farm.CreatedAt))
	return mapSQLError(err)
}

func (s *Store) CreateFarm(ctx context.Context, farm domain.Farm) error {
	return createFarm(ctx, s.q(), farm)
}
func (s *txStore) CreateFarm(ctx context.Context, farm domain.Farm) error {
	return createFarm(ctx, s.qer(), farm)
}

func getFarm(ctx context.Context, q queryer, tenantID, id string) (domain.Farm, error) {
	var farm domain.Farm
	var created string
	err := q.QueryRowContext(ctx, `SELECT id,tenant_id,name,village,owner_user_id,settlement_name,created_at FROM farms WHERE tenant_id=? AND id=?`, tenantID, id).
		Scan(&farm.ID, &farm.TenantID, &farm.Name, &farm.Village, &farm.OwnerUserID, &farm.SettlementName, &created)
	if err != nil {
		return domain.Farm{}, mapSQLError(err)
	}
	farm.CreatedAt, err = parseTime(created)
	return farm, err
}

func (s *Store) GetFarm(ctx context.Context, tenantID, id string) (domain.Farm, error) {
	return getFarm(ctx, s.q(), tenantID, id)
}
func (s *txStore) GetFarm(ctx context.Context, tenantID, id string) (domain.Farm, error) {
	return getFarm(ctx, s.qer(), tenantID, id)
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
func nullEmpty(value string) any {
	if value == "" {
		return nil
	}
	return value
}
func nullableTimeValue(value *time.Time) any {
	if value == nil {
		return nil
	}
	return formatTime(*value)
}

func requireAffected(result sql.Result, resource string) error {
	count, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read %s rows affected: %w", resource, err)
	}
	if count == 0 {
		return domain.ErrNotFound
	}
	return nil
}
