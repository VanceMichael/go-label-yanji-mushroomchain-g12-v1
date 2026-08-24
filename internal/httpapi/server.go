package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/VanceMichael/go-label-yanji-mushroomchain-g12-v1/internal/middleware"
	"github.com/VanceMichael/go-label-yanji-mushroomchain-g12-v1/internal/service"
)

type Readiness interface{ Ping(context.Context) error }
type API struct {
	auth        *service.AuthService
	batches     *service.BatchService
	orders      *service.OrderService
	settlements *service.SettlementService
	ready       Readiness
	logger      *slog.Logger
}

func New(authService *service.AuthService, batches *service.BatchService, orders *service.OrderService, settlements *service.SettlementService, ready Readiness, logger *slog.Logger) http.Handler {
	api := &API{auth: authService, batches: batches, orders: orders, settlements: settlements, ready: ready, logger: logger}
	root := http.NewServeMux()
	root.HandleFunc("GET /healthz", api.health)
	root.HandleFunc("GET /readyz", api.readiness)
	root.HandleFunc("POST /v1/auth/login", api.login)
	protected := http.NewServeMux()
	protected.HandleFunc("POST /v1/auth/logout", api.logout)
	protected.HandleFunc("GET /v1/batches", api.listBatches)
	protected.HandleFunc("POST /v1/batches", api.registerBatch)
	protected.HandleFunc("GET /v1/batches/{id}", api.getBatch)
	protected.HandleFunc("POST /v1/batches/{id}/inspections", api.inspectBatch)
	protected.HandleFunc("GET /v1/orders", api.listOrders)
	protected.HandleFunc("POST /v1/orders", api.createOrder)
	protected.HandleFunc("GET /v1/orders/{id}", api.getOrder)
	protected.HandleFunc("POST /v1/orders/{id}/allocate", api.allocateOrder)
	protected.HandleFunc("POST /v1/orders/{id}/deliver", api.deliverOrder)
	protected.HandleFunc("GET /v1/orders/{id}/settlements", api.listSettlements)
	protected.HandleFunc("POST /v1/settlements/{id}/approve", api.approveSettlement)
	protected.HandleFunc("POST /v1/settlements/{id}/pay", api.paySettlement)
	root.Handle("/v1/", middleware.Authenticate(authService, writeError, protected))
	return middleware.RequestContext(middleware.Recover(logger, middleware.AccessLog(logger, root)))
}

func (a *API) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
func (a *API) readiness(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := a.ready.Ping(ctx); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "not_ready"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}
