package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/VanceMichael/go-label-yanji-mushroomchain-g12-v1/internal/audit"
	"github.com/VanceMichael/go-label-yanji-mushroomchain-g12-v1/internal/clock"
	"github.com/VanceMichael/go-label-yanji-mushroomchain-g12-v1/internal/domain"
	"github.com/VanceMichael/go-label-yanji-mushroomchain-g12-v1/internal/pagination"
	"github.com/VanceMichael/go-label-yanji-mushroomchain-g12-v1/internal/repository"
	"github.com/VanceMichael/go-label-yanji-mushroomchain-g12-v1/internal/service"
	"github.com/VanceMichael/go-label-yanji-mushroomchain-g12-v1/internal/testkit"
)

type initialIdempotencyMissBarrier struct {
	repository.Tx

	mu      sync.Mutex
	reads   int
	release chan struct{}
}

type orderedCreateTransactor struct {
	repository.Transactor

	mu            sync.Mutex
	calls         int
	secondArrived chan struct{}
	firstDone     chan struct{}
}

func newOrderedCreateTransactor(tx repository.Transactor) *orderedCreateTransactor {
	return &orderedCreateTransactor{
		Transactor:    tx,
		secondArrived: make(chan struct{}),
		firstDone:     make(chan struct{}),
	}
}

func (t *orderedCreateTransactor) WithinTx(ctx context.Context, fn func(repository.Tx) error) error {
	t.mu.Lock()
	t.calls++
	call := t.calls
	if call == 2 {
		close(t.secondArrived)
	}
	t.mu.Unlock()

	switch call {
	case 1:
		select {
		case <-t.secondArrived:
		case <-ctx.Done():
			return ctx.Err()
		}
		err := t.Transactor.WithinTx(ctx, fn)
		close(t.firstDone)
		return err
	case 2:
		select {
		case <-t.firstDone:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return t.Transactor.WithinTx(ctx, fn)
}

func newInitialIdempotencyMissBarrier(repo repository.Tx) *initialIdempotencyMissBarrier {
	return &initialIdempotencyMissBarrier{Tx: repo, release: make(chan struct{})}
}

func (b *initialIdempotencyMissBarrier) FindOrderByIdempotency(ctx context.Context, tenantID, key string) (domain.Order, error) {
	b.mu.Lock()
	if b.reads >= 2 {
		b.mu.Unlock()
		return b.Tx.FindOrderByIdempotency(ctx, tenantID, key)
	}
	b.reads++
	if b.reads == 2 {
		close(b.release)
	}
	release := b.release
	b.mu.Unlock()

	select {
	case <-release:
		return domain.Order{}, domain.ErrNotFound
	case <-ctx.Done():
		return domain.Order{}, ctx.Err()
	}
}

func (b *initialIdempotencyMissBarrier) readCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.reads
}

type orderCreateHTTPResult struct {
	status int
	body   []byte
	err    error
}

func postOrder(handler http.Handler, token, key string, dueAt time.Time) orderCreateHTTPResult {
	payload, err := json.Marshal(map[string]any{
		"buyer_name":      "Helan Cooperative",
		"delivery_region": "Yinchuan",
		"due_at":          dueAt,
		"lines": []map[string]any{{
			"species":          "oyster",
			"quantity":         12,
			"unit_price_cents": 400,
		}},
	})
	if err != nil {
		return orderCreateHTTPResult{err: err}
	}
	request := httptest.NewRequest(http.MethodPost, "/v1/orders", bytes.NewReader(payload))
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", key)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return orderCreateHTTPResult{status: recorder.Code, body: recorder.Body.Bytes()}
}

func TestConcurrentIdempotentOrderCreateReturnsOneConfirmedOrder(t *testing.T) {
	data := testkit.New(t)
	fixed := clock.NewFixed(data.Now)
	authService := service.NewAuth(data.Store, fixed, time.Hour)
	barrierRepo := newInitialIdempotencyMissBarrier(data.Store)
	orderedTx := newOrderedCreateTransactor(data.Store)
	handler := New(
		authService,
		service.NewBatches(data.Store, data.Store, fixed),
		service.NewOrders(orderedTx, barrierRepo, fixed),
		service.NewSettlements(data.Store, data.Store, fixed),
		data.Store,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)
	fixture := apiFixture{data: data, handler: handler, auth: authService}
	token := fixture.login(t, domain.RoleDispatcher)

	start := make(chan struct{})
	results := make(chan orderCreateHTTPResult, 2)
	for range 2 {
		go func() {
			<-start
			results <- postOrder(handler, token, "shared-order-request", data.Now.Add(24*time.Hour))
		}()
	}
	close(start)
	first, second := <-results, <-results
	for index, result := range []orderCreateHTTPResult{first, second} {
		if result.err != nil {
			t.Fatalf("request %d failed: %v", index+1, result.err)
		}
		if result.status != http.StatusCreated {
			t.Fatalf("request %d status=%d body=%s idempotency_reads=%d", index+1, result.status, result.body, barrierRepo.readCount())
		}
	}

	var orders [2]domain.Order
	if err := json.Unmarshal(first.body, &orders[0]); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(second.body, &orders[1]); err != nil {
		t.Fatal(err)
	}
	if orders[0].ID == "" || orders[0].ID != orders[1].ID {
		t.Fatalf("concurrent calls returned different orders: %q and %q", orders[0].ID, orders[1].ID)
	}
	for index, order := range orders {
		if order.Status != domain.OrderConfirmed || order.Version != 2 || len(order.Lines) != 1 {
			t.Fatalf("request %d returned incomplete order: %+v", index+1, order)
		}
	}

	items, total, err := data.Store.ListOrders(context.Background(), repository.OrderFilter{
		TenantID: data.TenantID,
		Page:     pagination.Page{Number: 1, Size: 20},
	})
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(items) != 1 || len(items[0].Lines) != 1 || items[0].ID != orders[0].ID {
		t.Fatalf("persisted orders=%+v total=%d", items, total)
	}
	audits, auditTotal, err := data.Store.ListAudit(context.Background(), data.TenantID, "order", orders[0].ID, pagination.Page{Number: 1, Size: 20})
	if err != nil {
		t.Fatal(err)
	}
	if auditTotal != 1 || len(audits) != 1 || audits[0].Action != "order.create" || audits[0].Outcome != audit.OutcomeSucceeded {
		t.Fatalf("persisted audits=%+v total=%d", audits, auditTotal)
	}

	single := postOrder(handler, token, "single-order-request", data.Now.Add(48*time.Hour))
	if single.err != nil || single.status != http.StatusCreated {
		t.Fatalf("ordinary create status=%d err=%v body=%s", single.status, single.err, single.body)
	}
	items, total, err = data.Store.ListOrders(context.Background(), repository.OrderFilter{
		TenantID: data.TenantID,
		Page:     pagination.Page{Number: 1, Size: 20},
	})
	if err != nil || total != 2 || len(items) != 2 {
		t.Fatalf("ordinary create regression: orders=%+v total=%d err=%v", items, total, err)
	}
}
