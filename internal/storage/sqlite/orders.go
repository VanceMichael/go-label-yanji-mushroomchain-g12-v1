package sqlite

import (
	"context"
	"strings"
	"time"

	"github.com/VanceMichael/go-label-yanji-mushroomchain-g12-v1/internal/domain"
	"github.com/VanceMichael/go-label-yanji-mushroomchain-g12-v1/internal/repository"
)

const orderColumns = `id,tenant_id,buyer_name,delivery_region,status,idempotency_key,request_fingerprint,version,total_cents,requested_at,due_at,created_at,updated_at`

func createOrder(ctx context.Context, q queryer, order domain.Order) error {
	if err := order.Validate(); err != nil {
		return err
	}
	_, err := q.ExecContext(ctx, `INSERT INTO orders(`+orderColumns+`) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)`, order.ID,
		order.TenantID, order.BuyerName, order.DeliveryRegion, order.Status, order.IdempotencyKey, order.RequestFingerprint, order.Version,
		order.TotalCents, formatTime(order.RequestedAt), formatTime(order.DueAt), formatTime(order.CreatedAt), formatTime(order.UpdatedAt))
	if err != nil {
		return mapSQLError(err)
	}
	for _, line := range order.Lines {
		if _, err := q.ExecContext(ctx, `INSERT INTO order_lines(id,order_id,species,quantity,unit_price_cents) VALUES(?,?,?,?,?)`, line.ID, order.ID, line.Species, line.Quantity, line.UnitPriceCents); err != nil {
			return mapSQLError(err)
		}
	}
	return nil
}
func (s *Store) CreateOrder(ctx context.Context, o domain.Order) error {
	return createOrder(ctx, s.q(), o)
}
func (s *txStore) CreateOrder(ctx context.Context, o domain.Order) error {
	return createOrder(ctx, s.qer(), o)
}

func scanOrder(row interface{ Scan(...any) error }) (domain.Order, error) {
	var order domain.Order
	var requested, due, created, updated string
	err := row.Scan(&order.ID, &order.TenantID, &order.BuyerName, &order.DeliveryRegion, &order.Status,
		&order.IdempotencyKey, &order.RequestFingerprint, &order.Version, &order.TotalCents, &requested, &due, &created, &updated)
	if err != nil {
		return domain.Order{}, mapSQLError(err)
	}
	if order.RequestedAt, err = parseTime(requested); err != nil {
		return domain.Order{}, err
	}
	if order.DueAt, err = parseTime(due); err != nil {
		return domain.Order{}, err
	}
	if order.CreatedAt, err = parseTime(created); err != nil {
		return domain.Order{}, err
	}
	order.UpdatedAt, err = parseTime(updated)
	return order, err
}

func orderLines(ctx context.Context, q queryer, orderID string) ([]domain.OrderLine, error) {
	rows, err := q.QueryContext(ctx, `SELECT id,order_id,species,quantity,unit_price_cents FROM order_lines WHERE order_id=? ORDER BY id`, orderID)
	if err != nil {
		return nil, mapSQLError(err)
	}
	defer rows.Close()
	var lines []domain.OrderLine
	for rows.Next() {
		var line domain.OrderLine
		if err := rows.Scan(&line.ID, &line.OrderID, &line.Species, &line.Quantity, &line.UnitPriceCents); err != nil {
			return nil, err
		}
		lines = append(lines, line)
	}
	return lines, rows.Err()
}

func getOrder(ctx context.Context, q queryer, tenantID, id string) (domain.Order, error) {
	order, err := scanOrder(q.QueryRowContext(ctx, `SELECT `+orderColumns+` FROM orders WHERE tenant_id=? AND id=?`, tenantID, id))
	if err != nil {
		return domain.Order{}, err
	}
	order.Lines, err = orderLines(ctx, q, order.ID)
	return order, err
}
func (s *Store) GetOrder(ctx context.Context, tenantID, id string) (domain.Order, error) {
	return getOrder(ctx, s.q(), tenantID, id)
}
func (s *txStore) GetOrder(ctx context.Context, tenantID, id string) (domain.Order, error) {
	return getOrder(ctx, s.qer(), tenantID, id)
}

func findOrderByIdempotency(ctx context.Context, q queryer, tenantID, key string) (domain.Order, error) {
	order, err := scanOrder(q.QueryRowContext(ctx, `SELECT `+orderColumns+` FROM orders WHERE tenant_id=? AND idempotency_key=?`, tenantID, key))
	if err != nil {
		return domain.Order{}, err
	}
	order.Lines, err = orderLines(ctx, q, order.ID)
	return order, err
}
func (s *Store) FindOrderByIdempotency(ctx context.Context, tenantID, key string) (domain.Order, error) {
	return findOrderByIdempotency(ctx, s.q(), tenantID, key)
}
func (s *txStore) FindOrderByIdempotency(ctx context.Context, tenantID, key string) (domain.Order, error) {
	return findOrderByIdempotency(ctx, s.qer(), tenantID, key)
}

func listOrders(ctx context.Context, q queryer, filter repository.OrderFilter) ([]domain.Order, int, error) {
	where, args := []string{"tenant_id=?"}, []any{filter.TenantID}
	if filter.Status != "" {
		where, args = append(where, "status=?"), append(args, filter.Status)
	}
	if filter.DueBefore != nil {
		where, args = append(where, "due_at<=?"), append(args, formatTime(*filter.DueBefore))
	}
	clause := strings.Join(where, " AND ")
	var total int
	if err := q.QueryRowContext(ctx, `SELECT COUNT(*) FROM orders WHERE `+clause, args...).Scan(&total); err != nil {
		return nil, 0, mapSQLError(err)
	}
	rows, err := q.QueryContext(ctx, `SELECT `+orderColumns+` FROM orders WHERE `+clause+` ORDER BY due_at,id LIMIT ? OFFSET ?`, append(args, filter.Page.Size, filter.Page.Offset())...)
	if err != nil {
		return nil, 0, mapSQLError(err)
	}
	defer rows.Close()
	var items []domain.Order
	for rows.Next() {
		item, scanErr := scanOrder(rows)
		if scanErr != nil {
			return nil, 0, scanErr
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	for index := range items {
		items[index].Lines, err = orderLines(ctx, q, items[index].ID)
		if err != nil {
			return nil, 0, err
		}
	}
	return items, total, nil
}
func (s *Store) ListOrders(ctx context.Context, f repository.OrderFilter) ([]domain.Order, int, error) {
	return listOrders(ctx, s.q(), f)
}
func (s *txStore) ListOrders(ctx context.Context, f repository.OrderFilter) ([]domain.Order, int, error) {
	return listOrders(ctx, s.qer(), f)
}

func transitionOrder(ctx context.Context, q queryer, tenantID, id string, version int64, to domain.OrderStatus, at time.Time) error {
	current, err := getOrder(ctx, q, tenantID, id)
	if err != nil {
		return err
	}
	if !current.Status.CanTransition(to) {
		return domain.StateError{Entity: "order", From: string(current.Status), To: string(to)}
	}
	result, err := q.ExecContext(ctx, `UPDATE orders SET status=?,version=version+1,updated_at=? WHERE tenant_id=? AND id=? AND version=?`, to, formatTime(at), tenantID, id, version)
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
func (s *Store) TransitionOrder(ctx context.Context, tenantID, id string, version int64, to domain.OrderStatus, at time.Time) error {
	return transitionOrder(ctx, s.q(), tenantID, id, version, to, at)
}
func (s *txStore) TransitionOrder(ctx context.Context, tenantID, id string, version int64, to domain.OrderStatus, at time.Time) error {
	return transitionOrder(ctx, s.qer(), tenantID, id, version, to, at)
}

func createAllocation(ctx context.Context, q queryer, item domain.InventoryAllocation) error {
	_, err := q.ExecContext(ctx, `INSERT INTO inventory_allocations(id,tenant_id,order_id,line_id,batch_id,quantity,created_at) VALUES(?,?,?,?,?,?,?)`, item.ID, item.TenantID, item.OrderID, item.LineID, item.BatchID, item.Quantity, formatTime(item.CreatedAt))
	return mapSQLError(err)
}
func (s *Store) CreateAllocation(ctx context.Context, item domain.InventoryAllocation) error {
	return createAllocation(ctx, s.q(), item)
}
func (s *txStore) CreateAllocation(ctx context.Context, item domain.InventoryAllocation) error {
	return createAllocation(ctx, s.qer(), item)
}

func listAllocations(ctx context.Context, q queryer, tenantID, orderID string) ([]domain.InventoryAllocation, error) {
	rows, err := q.QueryContext(ctx, `SELECT id,tenant_id,order_id,line_id,batch_id,quantity,created_at FROM inventory_allocations WHERE tenant_id=? AND order_id=? ORDER BY line_id,batch_id`, tenantID, orderID)
	if err != nil {
		return nil, mapSQLError(err)
	}
	defer rows.Close()
	var items []domain.InventoryAllocation
	for rows.Next() {
		var item domain.InventoryAllocation
		var created string
		if err := rows.Scan(&item.ID, &item.TenantID, &item.OrderID, &item.LineID, &item.BatchID, &item.Quantity, &created); err != nil {
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
func (s *Store) ListAllocations(ctx context.Context, t, o string) ([]domain.InventoryAllocation, error) {
	return listAllocations(ctx, s.q(), t, o)
}
func (s *txStore) ListAllocations(ctx context.Context, t, o string) ([]domain.InventoryAllocation, error) {
	return listAllocations(ctx, s.qer(), t, o)
}
