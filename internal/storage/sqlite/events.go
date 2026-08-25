package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/VanceMichael/go-label-yanji-mushroomchain-g12-v1/internal/audit"
	"github.com/VanceMichael/go-label-yanji-mushroomchain-g12-v1/internal/domain"
	"github.com/VanceMichael/go-label-yanji-mushroomchain-g12-v1/internal/pagination"
)

func createOutbox(ctx context.Context, q queryer, v domain.OutboxEvent) error {
	_, err := q.ExecContext(ctx, `INSERT INTO outbox_events(id,tenant_id,aggregate_type,aggregate_id,event_type,payload,status,attempts,available_at,lease_owner,lease_until,last_error,created_at,delivered_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, v.ID, v.TenantID, v.AggregateType, v.AggregateID, v.EventType, v.Payload, v.Status, v.Attempts, formatTime(v.AvailableAt), v.LeaseOwner, nullableTimeValue(v.LeaseUntil), v.LastError, formatTime(v.CreatedAt), nullableTimeValue(v.DeliveredAt))
	return mapSQLError(err)
}
func (s *Store) CreateOutboxEvent(ctx context.Context, v domain.OutboxEvent) error {
	return createOutbox(ctx, s.q(), v)
}
func (s *txStore) CreateOutboxEvent(ctx context.Context, v domain.OutboxEvent) error {
	return createOutbox(ctx, s.qer(), v)
}

const outboxColumns = `id,tenant_id,aggregate_type,aggregate_id,event_type,payload,status,attempts,available_at,lease_owner,lease_until,last_error,created_at,delivered_at`

func scanOutbox(row interface{ Scan(...any) error }) (domain.OutboxEvent, error) {
	var v domain.OutboxEvent
	var available, created string
	var lease, delivered sql.NullString
	err := row.Scan(&v.ID, &v.TenantID, &v.AggregateType, &v.AggregateID, &v.EventType, &v.Payload, &v.Status, &v.Attempts, &available, &v.LeaseOwner, &lease, &v.LastError, &created, &delivered)
	if err != nil {
		return v, mapSQLError(err)
	}
	v.AvailableAt, err = parseTime(available)
	if err != nil {
		return v, err
	}
	v.CreatedAt, err = parseTime(created)
	if err != nil {
		return v, err
	}
	v.LeaseUntil, err = nullableTime(lease)
	if err != nil {
		return v, err
	}
	v.DeliveredAt, err = nullableTime(delivered)
	return v, err
}
func claimOutbox(ctx context.Context, q queryer, owner string, now, until time.Time) (domain.OutboxEvent, error) {
	return scanOutbox(q.QueryRowContext(ctx, `UPDATE outbox_events SET status='claimed',lease_owner=?,lease_until=?,attempts=attempts+1 WHERE id=(SELECT id FROM outbox_events WHERE available_at<=? AND (status='pending' OR (status='claimed' AND lease_until<?)) ORDER BY available_at,id LIMIT 1) RETURNING `+outboxColumns, owner, formatTime(until), formatTime(now), formatTime(now)))
}
func (s *Store) ClaimOutbox(ctx context.Context, o string, n, u time.Time) (domain.OutboxEvent, error) {
	return claimOutbox(ctx, s.q(), o, n, u)
}
func (s *txStore) ClaimOutbox(ctx context.Context, o string, n, u time.Time) (domain.OutboxEvent, error) {
	return claimOutbox(ctx, s.qer(), o, n, u)
}
func completeOutbox(ctx context.Context, q queryer, id, owner string, at time.Time) error {
	r, e := q.ExecContext(ctx, `UPDATE outbox_events SET status='delivered',delivered_at=?,lease_owner='',lease_until=NULL WHERE id=? AND lease_owner=? AND status='claimed'`, formatTime(at), id, owner)
	if e != nil {
		return mapSQLError(e)
	}
	return requireAffected(r, "outbox lease")
}
func (s *Store) CompleteOutbox(ctx context.Context, id, o string, at time.Time) error {
	return completeOutbox(ctx, s.q(), id, o, at)
}
func (s *txStore) CompleteOutbox(ctx context.Context, id, o string, at time.Time) error {
	return completeOutbox(ctx, s.qer(), id, o, at)
}
func failOutbox(ctx context.Context, q queryer, id, owner, message string, max int, next time.Time) error {
	status := "pending"
	if max <= 0 {
		status = "dead"
	}
	r, e := q.ExecContext(ctx, `UPDATE outbox_events SET status=?,last_error=?,available_at=?,lease_owner='',lease_until=NULL WHERE id=? AND lease_owner=? AND status='claimed'`, status, message, formatTime(next), id, owner)
	if e != nil {
		return mapSQLError(e)
	}
	return requireAffected(r, "outbox lease")
}
func (s *Store) FailOutbox(ctx context.Context, id, o, m string, max int, n time.Time) error {
	return failOutbox(ctx, s.q(), id, o, m, max, n)
}
func (s *txStore) FailOutbox(ctx context.Context, id, o, m string, max int, n time.Time) error {
	return failOutbox(ctx, s.qer(), id, o, m, max, n)
}

func createAudit(ctx context.Context, q queryer, e audit.Event) error {
	if err := e.Validate(); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("audit write canceled: %w", err)
	}
	_, err := q.ExecContext(ctx, `INSERT INTO audit_logs(id,tenant_id,actor_id,request_id,action,object_type,object_id,outcome,metadata,created_at) VALUES(?,?,?,?,?,?,?,?,?,?)`, e.ID, e.TenantID, e.ActorID, e.RequestID, e.Action, e.Object, e.ObjectID, e.Outcome, []byte(e.Metadata), formatTime(e.CreatedAt))
	return mapSQLError(err)
}
func (s *Store) CreateAudit(ctx context.Context, e audit.Event) error {
	return createAudit(ctx, s.q(), e)
}
func (s *txStore) CreateAudit(ctx context.Context, e audit.Event) error {
	return createAudit(ctx, s.qer(), e)
}
func listAudit(ctx context.Context, q queryer, t, object, id string, page pagination.Page) ([]audit.Event, int, error) {
	var total int
	if err := q.QueryRowContext(ctx, `SELECT COUNT(*) FROM audit_logs WHERE tenant_id=? AND object_type=? AND object_id=?`, t, object, id).Scan(&total); err != nil {
		return nil, 0, mapSQLError(err)
	}
	rows, err := q.QueryContext(ctx, `SELECT id,tenant_id,actor_id,request_id,action,object_type,object_id,outcome,metadata,created_at FROM audit_logs WHERE tenant_id=? AND object_type=? AND object_id=? ORDER BY created_at DESC,id LIMIT ? OFFSET ?`, t, object, id, page.Size, page.Offset())
	if err != nil {
		return nil, 0, mapSQLError(err)
	}
	defer rows.Close()
	var items []audit.Event
	for rows.Next() {
		var e audit.Event
		var metadata []byte
		var created string
		if err := rows.Scan(&e.ID, &e.TenantID, &e.ActorID, &e.RequestID, &e.Action, &e.Object, &e.ObjectID, &e.Outcome, &metadata, &created); err != nil {
			return nil, 0, err
		}
		e.Metadata = metadata
		e.CreatedAt, err = parseTime(created)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, e)
	}
	return items, total, rows.Err()
}
func (s *Store) ListAudit(ctx context.Context, t, o, id string, p pagination.Page) ([]audit.Event, int, error) {
	return listAudit(ctx, s.q(), t, o, id, p)
}
func (s *txStore) ListAudit(ctx context.Context, t, o, id string, p pagination.Page) ([]audit.Event, int, error) {
	return listAudit(ctx, s.qer(), t, o, id, p)
}
