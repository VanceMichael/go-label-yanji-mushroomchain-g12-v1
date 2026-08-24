package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/VanceMichael/go-label-yanji-mushroomchain-g12-v1/internal/domain"
)

func createShipment(ctx context.Context, q queryer, v domain.Shipment) error {
	_, err := q.ExecContext(ctx, `INSERT INTO shipments(id,tenant_id,order_id,status,carrier,tracking_number,claimed_by,lease_until,attempts,last_error,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`, v.ID, v.TenantID, v.OrderID, v.Status, v.Carrier, v.TrackingNumber, v.ClaimedBy, nullableTimeValue(v.LeaseUntil), v.Attempts, v.LastError, formatTime(v.CreatedAt), formatTime(v.UpdatedAt))
	return mapSQLError(err)
}
func (s *Store) CreateShipment(ctx context.Context, v domain.Shipment) error {
	return createShipment(ctx, s.q(), v)
}
func (s *txStore) CreateShipment(ctx context.Context, v domain.Shipment) error {
	return createShipment(ctx, s.qer(), v)
}

func scanShipment(row interface{ Scan(...any) error }) (domain.Shipment, error) {
	var v domain.Shipment
	var lease sql.NullString
	var created, updated string
	err := row.Scan(&v.ID, &v.TenantID, &v.OrderID, &v.Status, &v.Carrier, &v.TrackingNumber, &v.ClaimedBy, &lease, &v.Attempts, &v.LastError, &created, &updated)
	if err != nil {
		return v, mapSQLError(err)
	}
	v.LeaseUntil, err = nullableTime(lease)
	if err != nil {
		return v, err
	}
	v.CreatedAt, err = parseTime(created)
	if err != nil {
		return v, err
	}
	v.UpdatedAt, err = parseTime(updated)
	return v, err
}

const shipmentColumns = `id,tenant_id,order_id,status,carrier,tracking_number,claimed_by,lease_until,attempts,last_error,created_at,updated_at`

func getShipmentByOrder(ctx context.Context, q queryer, t, o string) (domain.Shipment, error) {
	return scanShipment(q.QueryRowContext(ctx, `SELECT `+shipmentColumns+` FROM shipments WHERE tenant_id=? AND order_id=?`, t, o))
}
func (s *Store) GetShipmentByOrder(ctx context.Context, t, o string) (domain.Shipment, error) {
	return getShipmentByOrder(ctx, s.q(), t, o)
}
func (s *txStore) GetShipmentByOrder(ctx context.Context, t, o string) (domain.Shipment, error) {
	return getShipmentByOrder(ctx, s.qer(), t, o)
}

func claimShipment(ctx context.Context, q queryer, owner string, now, until time.Time) (domain.Shipment, error) {
	row := q.QueryRowContext(ctx, `UPDATE shipments SET status='claimed',claimed_by=?,lease_until=?,attempts=attempts+1,updated_at=? WHERE id=(SELECT id FROM shipments WHERE (status='pending' OR (status='claimed' AND lease_until<?) OR status='failed') ORDER BY updated_at,id LIMIT 1) RETURNING `+shipmentColumns, owner, formatTime(until), formatTime(now), formatTime(now))
	return scanShipment(row)
}
func (s *Store) ClaimShipment(ctx context.Context, o string, n, u time.Time) (domain.Shipment, error) {
	return claimShipment(ctx, s.q(), o, n, u)
}
func (s *txStore) ClaimShipment(ctx context.Context, o string, n, u time.Time) (domain.Shipment, error) {
	return claimShipment(ctx, s.qer(), o, n, u)
}

func completeShipment(ctx context.Context, q queryer, id, owner string, status domain.ShipmentStatus, lastError string, at time.Time) error {
	result, err := q.ExecContext(ctx, `UPDATE shipments SET status=?,last_error=?,lease_until=NULL,claimed_by='',updated_at=? WHERE id=? AND claimed_by=? AND status='claimed'`, status, lastError, formatTime(at), id, owner)
	if err != nil {
		return mapSQLError(err)
	}
	return requireAffected(result, "shipment lease")
}
func (s *Store) CompleteShipment(ctx context.Context, id, o string, st domain.ShipmentStatus, e string, at time.Time) error {
	return completeShipment(ctx, s.q(), id, o, st, e, at)
}
func (s *txStore) CompleteShipment(ctx context.Context, id, o string, st domain.ShipmentStatus, e string, at time.Time) error {
	return completeShipment(ctx, s.qer(), id, o, st, e, at)
}

func createSettlement(ctx context.Context, q queryer, v domain.Settlement) error {
	_, err := q.ExecContext(ctx, `INSERT INTO settlements(id,tenant_id,farm_id,order_id,amount_cents,status,version,approved_by,approved_at,paid_at,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`, v.ID, v.TenantID, v.FarmID, v.OrderID, v.AmountCents, v.Status, v.Version, v.ApprovedBy, nullableTimeValue(v.ApprovedAt), nullableTimeValue(v.PaidAt), formatTime(v.CreatedAt), formatTime(v.UpdatedAt))
	return mapSQLError(err)
}
func (s *Store) CreateSettlement(ctx context.Context, v domain.Settlement) error {
	return createSettlement(ctx, s.q(), v)
}
func (s *txStore) CreateSettlement(ctx context.Context, v domain.Settlement) error {
	return createSettlement(ctx, s.qer(), v)
}

func scanSettlement(row interface{ Scan(...any) error }) (domain.Settlement, error) {
	var v domain.Settlement
	var approved, paid sql.NullString
	var created, updated string
	err := row.Scan(&v.ID, &v.TenantID, &v.FarmID, &v.OrderID, &v.AmountCents, &v.Status, &v.Version, &v.ApprovedBy, &approved, &paid, &created, &updated)
	if err != nil {
		return v, mapSQLError(err)
	}
	v.ApprovedAt, err = nullableTime(approved)
	if err != nil {
		return v, err
	}
	v.PaidAt, err = nullableTime(paid)
	if err != nil {
		return v, err
	}
	v.CreatedAt, err = parseTime(created)
	if err != nil {
		return v, err
	}
	v.UpdatedAt, err = parseTime(updated)
	return v, err
}

const settlementColumns = `id,tenant_id,farm_id,order_id,amount_cents,status,version,approved_by,approved_at,paid_at,created_at,updated_at`

func getSettlement(ctx context.Context, q queryer, t, id string) (domain.Settlement, error) {
	return scanSettlement(q.QueryRowContext(ctx, `SELECT `+settlementColumns+` FROM settlements WHERE tenant_id=? AND id=?`, t, id))
}
func (s *Store) GetSettlement(ctx context.Context, t, id string) (domain.Settlement, error) {
	return getSettlement(ctx, s.q(), t, id)
}
func (s *txStore) GetSettlement(ctx context.Context, t, id string) (domain.Settlement, error) {
	return getSettlement(ctx, s.qer(), t, id)
}
func listSettlementsByOrder(ctx context.Context, q queryer, t, o string) ([]domain.Settlement, error) {
	rows, err := q.QueryContext(ctx, `SELECT `+settlementColumns+` FROM settlements WHERE tenant_id=? AND order_id=? ORDER BY farm_id`, t, o)
	if err != nil {
		return nil, mapSQLError(err)
	}
	defer rows.Close()
	var items []domain.Settlement
	for rows.Next() {
		v, e := scanSettlement(rows)
		if e != nil {
			return nil, e
		}
		items = append(items, v)
	}
	return items, rows.Err()
}
func (s *Store) ListSettlementsByOrder(ctx context.Context, t, o string) ([]domain.Settlement, error) {
	return listSettlementsByOrder(ctx, s.q(), t, o)
}
func (s *txStore) ListSettlementsByOrder(ctx context.Context, t, o string) ([]domain.Settlement, error) {
	return listSettlementsByOrder(ctx, s.qer(), t, o)
}

func approveSettlement(ctx context.Context, q queryer, t, id string, version int64, actor string, at time.Time) error {
	result, err := q.ExecContext(ctx, `UPDATE settlements SET status='approved',version=version+1,approved_by=?,approved_at=?,updated_at=? WHERE tenant_id=? AND id=? AND status='pending' AND version=?`, actor, formatTime(at), formatTime(at), t, id, version)
	if err != nil {
		return mapSQLError(err)
	}
	count, e := result.RowsAffected()
	if e != nil {
		return e
	}
	if count == 0 {
		return domain.ErrVersionConflict
	}
	return nil
}
func (s *Store) ApproveSettlement(ctx context.Context, t, id string, v int64, a string, at time.Time) error {
	return approveSettlement(ctx, s.q(), t, id, v, a, at)
}
func (s *txStore) ApproveSettlement(ctx context.Context, t, id string, v int64, a string, at time.Time) error {
	return approveSettlement(ctx, s.qer(), t, id, v, a, at)
}
func markSettlementPaid(ctx context.Context, q queryer, t, id string, version int64, at time.Time) error {
	result, err := q.ExecContext(ctx, `UPDATE settlements SET status='paid',version=version+1,paid_at=?,updated_at=? WHERE tenant_id=? AND id=? AND status='approved' AND version=?`, formatTime(at), formatTime(at), t, id, version)
	if err != nil {
		return mapSQLError(err)
	}
	count, e := result.RowsAffected()
	if e != nil {
		return e
	}
	if count == 0 {
		return domain.ErrVersionConflict
	}
	return nil
}
func (s *Store) MarkSettlementPaid(ctx context.Context, t, id string, v int64, at time.Time) error {
	return markSettlementPaid(ctx, s.q(), t, id, v, at)
}
func (s *txStore) MarkSettlementPaid(ctx context.Context, t, id string, v int64, at time.Time) error {
	return markSettlementPaid(ctx, s.qer(), t, id, v, at)
}

var _ = errors.Is
