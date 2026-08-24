package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/VanceMichael/go-label-yanji-mushroomchain-g12-v1/internal/domain"
	"github.com/VanceMichael/go-label-yanji-mushroomchain-g12-v1/internal/repository"
)

const batchColumns = `id,tenant_id,farm_id,code,species,produced_at,expires_at,quantity_produced,quantity_available,unit_price_cents,status,version,created_at,updated_at`

func createBatch(ctx context.Context, q queryer, batch domain.SubstrateBatch) error {
	if err := batch.Validate(); err != nil {
		return err
	}
	_, err := q.ExecContext(ctx, `INSERT INTO substrate_batches(`+batchColumns+`) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		batch.ID, batch.TenantID, batch.FarmID, batch.Code, batch.Species, formatTime(batch.ProducedAt), formatTime(batch.ExpiresAt),
		batch.QuantityProduced, batch.QuantityAvailable, batch.UnitPriceCents, batch.Status, batch.Version,
		formatTime(batch.CreatedAt), formatTime(batch.UpdatedAt))
	return mapSQLError(err)
}

func (s *Store) CreateBatch(ctx context.Context, batch domain.SubstrateBatch) error {
	return createBatch(ctx, s.q(), batch)
}
func (s *txStore) CreateBatch(ctx context.Context, batch domain.SubstrateBatch) error {
	return createBatch(ctx, s.qer(), batch)
}

func scanBatch(row interface{ Scan(...any) error }) (domain.SubstrateBatch, error) {
	var batch domain.SubstrateBatch
	var produced, expires, created, updated string
	err := row.Scan(&batch.ID, &batch.TenantID, &batch.FarmID, &batch.Code, &batch.Species, &produced, &expires,
		&batch.QuantityProduced, &batch.QuantityAvailable, &batch.UnitPriceCents, &batch.Status, &batch.Version, &created, &updated)
	if err != nil {
		return domain.SubstrateBatch{}, mapSQLError(err)
	}
	var parseErr error
	if batch.ProducedAt, parseErr = parseTime(produced); parseErr != nil {
		return domain.SubstrateBatch{}, parseErr
	}
	if batch.ExpiresAt, parseErr = parseTime(expires); parseErr != nil {
		return domain.SubstrateBatch{}, parseErr
	}
	if batch.CreatedAt, parseErr = parseTime(created); parseErr != nil {
		return domain.SubstrateBatch{}, parseErr
	}
	batch.UpdatedAt, parseErr = parseTime(updated)
	return batch, parseErr
}

func getBatch(ctx context.Context, q queryer, tenantID, id string) (domain.SubstrateBatch, error) {
	return scanBatch(q.QueryRowContext(ctx, `SELECT `+batchColumns+` FROM substrate_batches WHERE tenant_id=? AND id=?`, tenantID, id))
}

func (s *Store) GetBatch(ctx context.Context, tenantID, id string) (domain.SubstrateBatch, error) {
	return getBatch(ctx, s.q(), tenantID, id)
}
func (s *txStore) GetBatch(ctx context.Context, tenantID, id string) (domain.SubstrateBatch, error) {
	return getBatch(ctx, s.qer(), tenantID, id)
}

func listBatches(ctx context.Context, q queryer, filter repository.BatchFilter) ([]domain.SubstrateBatch, int, error) {
	where := []string{"tenant_id=?"}
	args := []any{filter.TenantID}
	if filter.FarmID != "" {
		where, args = append(where, "farm_id=?"), append(args, filter.FarmID)
	}
	if filter.Species != "" {
		where, args = append(where, "species=?"), append(args, filter.Species)
	}
	if filter.Status != "" {
		where, args = append(where, "status=?"), append(args, filter.Status)
	}
	clause := strings.Join(where, " AND ")
	var total int
	if err := q.QueryRowContext(ctx, `SELECT COUNT(*) FROM substrate_batches WHERE `+clause, args...).Scan(&total); err != nil {
		return nil, 0, mapSQLError(err)
	}
	pageArgs := append(append([]any{}, args...), filter.Page.Size, filter.Page.Offset())
	rows, err := q.QueryContext(ctx, `SELECT `+batchColumns+` FROM substrate_batches WHERE `+clause+` ORDER BY expires_at ASC,produced_at ASC,id LIMIT ? OFFSET ?`, pageArgs...)
	if err != nil {
		return nil, 0, mapSQLError(err)
	}
	defer rows.Close()
	items := make([]domain.SubstrateBatch, 0)
	for rows.Next() {
		item, scanErr := scanBatch(rows)
		if scanErr != nil {
			return nil, 0, scanErr
		}
		items = append(items, item)
	}
	return items, total, rows.Err()
}

func (s *Store) ListBatches(ctx context.Context, f repository.BatchFilter) ([]domain.SubstrateBatch, int, error) {
	return listBatches(ctx, s.q(), f)
}
func (s *txStore) ListBatches(ctx context.Context, f repository.BatchFilter) ([]domain.SubstrateBatch, int, error) {
	return listBatches(ctx, s.qer(), f)
}

func transitionBatch(ctx context.Context, q queryer, tenantID, id string, version int64, to domain.BatchStatus, at time.Time) error {
	current, err := getBatch(ctx, q, tenantID, id)
	if err != nil {
		return err
	}
	if !current.Status.CanTransition(to) {
		return domain.StateError{Entity: "batch", From: string(current.Status), To: string(to)}
	}
	result, err := q.ExecContext(ctx, `UPDATE substrate_batches SET status=?,version=version+1,updated_at=? WHERE tenant_id=? AND id=? AND version=?`, to, formatTime(at), tenantID, id, version)
	if err != nil {
		return mapSQLError(err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return domain.ErrVersionConflict
	}
	return nil
}

func (s *Store) TransitionBatch(ctx context.Context, tenantID, id string, version int64, to domain.BatchStatus, at time.Time) error {
	return transitionBatch(ctx, s.q(), tenantID, id, version, to, at)
}
func (s *txStore) TransitionBatch(ctx context.Context, tenantID, id string, version int64, to domain.BatchStatus, at time.Time) error {
	return transitionBatch(ctx, s.qer(), tenantID, id, version, to, at)
}

func createInspection(ctx context.Context, q queryer, inspection domain.QualityInspection) error {
	_, err := q.ExecContext(ctx, `INSERT INTO quality_inspections(id,tenant_id,batch_id,inspector_id,decision,moisture_bp,sample_count,notes,inspected_at,created_at) VALUES(?,?,?,?,?,?,?,?,?,?)`,
		inspection.ID, inspection.TenantID, inspection.BatchID, inspection.InspectorID, inspection.Decision, inspection.MoistureBP,
		inspection.SampleCount, inspection.Notes, formatTime(inspection.InspectedAt), formatTime(inspection.CreatedAt))
	return mapSQLError(err)
}

func (s *Store) CreateInspection(ctx context.Context, i domain.QualityInspection) error {
	return createInspection(ctx, s.q(), i)
}
func (s *txStore) CreateInspection(ctx context.Context, i domain.QualityInspection) error {
	return createInspection(ctx, s.qer(), i)
}

func listInspections(ctx context.Context, q queryer, tenantID, batchID string) ([]domain.QualityInspection, error) {
	rows, err := q.QueryContext(ctx, `SELECT id,tenant_id,batch_id,inspector_id,decision,moisture_bp,sample_count,notes,inspected_at,created_at FROM quality_inspections WHERE tenant_id=? AND batch_id=? ORDER BY inspected_at,id`, tenantID, batchID)
	if err != nil {
		return nil, mapSQLError(err)
	}
	defer rows.Close()
	var items []domain.QualityInspection
	for rows.Next() {
		var item domain.QualityInspection
		var inspected, created string
		if err := rows.Scan(&item.ID, &item.TenantID, &item.BatchID, &item.InspectorID, &item.Decision, &item.MoistureBP, &item.SampleCount, &item.Notes, &inspected, &created); err != nil {
			return nil, err
		}
		item.InspectedAt, err = parseTime(inspected)
		if err != nil {
			return nil, err
		}
		item.CreatedAt, err = parseTime(created)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) ListInspections(ctx context.Context, tenantID, batchID string) ([]domain.QualityInspection, error) {
	return listInspections(ctx, s.q(), tenantID, batchID)
}
func (s *txStore) ListInspections(ctx context.Context, tenantID, batchID string) ([]domain.QualityInspection, error) {
	return listInspections(ctx, s.qer(), tenantID, batchID)
}

func allocateBatch(ctx context.Context, q queryer, tenantID, id string, quantity, version int64, at time.Time) error {
	if quantity <= 0 {
		return domain.FieldError{Field: "quantity", Message: "must be positive"}
	}
	result, err := q.ExecContext(ctx, `UPDATE substrate_batches SET quantity_available=quantity_available-?,version=version+1,updated_at=?,status=CASE WHEN quantity_available-?=0 THEN 'exhausted' ELSE status END WHERE tenant_id=? AND id=? AND status='released' AND version=? AND quantity_available>=?`, quantity, formatTime(at), quantity, tenantID, id, version, quantity)
	if err != nil {
		return mapSQLError(err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		batch, getErr := getBatch(ctx, q, tenantID, id)
		if getErr != nil {
			return getErr
		}
		if batch.Version != version {
			return domain.ErrVersionConflict
		}
		return domain.ErrCapacity
	}
	return nil
}

func (s *Store) AllocateBatch(ctx context.Context, tenantID, id string, quantity, version int64, at time.Time) error {
	return allocateBatch(ctx, s.q(), tenantID, id, quantity, version, at)
}
func (s *txStore) AllocateBatch(ctx context.Context, tenantID, id string, quantity, version int64, at time.Time) error {
	return allocateBatch(ctx, s.qer(), tenantID, id, quantity, version, at)
}

func restoreBatch(ctx context.Context, q queryer, tenantID, id string, quantity int64, at time.Time) error {
	result, err := q.ExecContext(ctx, `UPDATE substrate_batches SET quantity_available=quantity_available+?,version=version+1,updated_at=?,status=CASE WHEN status='exhausted' THEN 'released' ELSE status END WHERE tenant_id=? AND id=? AND quantity_available+?<=quantity_produced`, quantity, formatTime(at), tenantID, id, quantity)
	if err != nil {
		return mapSQLError(err)
	}
	if err := requireAffected(result, "batch"); err != nil {
		return fmt.Errorf("restore batch: %w", err)
	}
	return nil
}

func (s *Store) RestoreBatch(ctx context.Context, tenantID, id string, quantity int64, at time.Time) error {
	return restoreBatch(ctx, s.q(), tenantID, id, quantity, at)
}
func (s *txStore) RestoreBatch(ctx context.Context, tenantID, id string, quantity int64, at time.Time) error {
	return restoreBatch(ctx, s.qer(), tenantID, id, quantity, at)
}

var _ = sql.ErrNoRows
