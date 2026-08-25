package httpapi

import (
	"net/http"
	"time"

	"github.com/VanceMichael/go-label-yanji-mushroomchain-g12-v1/internal/domain"
	"github.com/VanceMichael/go-label-yanji-mushroomchain-g12-v1/internal/middleware"
	"github.com/VanceMichael/go-label-yanji-mushroomchain-g12-v1/internal/pagination"
	"github.com/VanceMichael/go-label-yanji-mushroomchain-g12-v1/internal/repository"
	"github.com/VanceMichael/go-label-yanji-mushroomchain-g12-v1/internal/service"
)

func (a *API) createOrder(w http.ResponseWriter, r *http.Request) {
	var body struct {
		BuyerName      string             `json:"buyer_name"`
		DeliveryRegion string             `json:"delivery_region"`
		DueAt          time.Time          `json:"due_at"`
		Lines          []domain.OrderLine `json:"lines"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, r, err)
		return
	}
	input := service.CreateOrderInput{BuyerName: body.BuyerName, DeliveryRegion: body.DeliveryRegion, DueAt: body.DueAt, Lines: body.Lines, IdempotencyKey: r.Header.Get("Idempotency-Key"), RequestID: middleware.RequestID(r.Context())}
	input.RequestFingerprint = service.FingerprintOrderRequest(input)
	order, err := a.orders.Create(r.Context(), input)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, order)
}
func (a *API) getOrder(w http.ResponseWriter, r *http.Request) {
	order, err := a.orders.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, order)
}
func (a *API) listOrders(w http.ResponseWriter, r *http.Request) {
	page, err := pagination.Parse(r.URL.Query().Get("page"), r.URL.Query().Get("page_size"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	result, err := a.orders.List(r.Context(), repository.OrderFilter{Status: domain.OrderStatus(r.URL.Query().Get("status")), Page: page})
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}
func (a *API) allocateOrder(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ExpectedVersion int64 `json:"expected_version"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, r, err)
		return
	}
	order, err := a.orders.Allocate(r.Context(), service.AllocateInput{OrderID: r.PathValue("id"), ExpectedVersion: body.ExpectedVersion, RequestID: middleware.RequestID(r.Context())})
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, order)
}
func (a *API) deliverOrder(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ExpectedVersion int64 `json:"expected_version"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, r, err)
		return
	}
	order, err := a.orders.MarkDelivered(r.Context(), service.DeliveryInput{OrderID: r.PathValue("id"), ExpectedVersion: body.ExpectedVersion, RequestID: middleware.RequestID(r.Context())})
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, order)
}
func (a *API) listSettlements(w http.ResponseWriter, r *http.Request) {
	items, err := a.settlements.ListForOrder(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}
func (a *API) approveSettlement(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ExpectedVersion int64 `json:"expected_version"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, r, err)
		return
	}
	item, err := a.settlements.Approve(r.Context(), service.ApproveSettlementInput{SettlementID: r.PathValue("id"), ExpectedVersion: body.ExpectedVersion, RequestID: middleware.RequestID(r.Context())})
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}
func (a *API) paySettlement(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ExpectedVersion int64 `json:"expected_version"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, r, err)
		return
	}
	item, err := a.settlements.MarkPaid(r.Context(), service.PaySettlementInput{SettlementID: r.PathValue("id"), ExpectedVersion: body.ExpectedVersion, RequestID: middleware.RequestID(r.Context())})
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}
