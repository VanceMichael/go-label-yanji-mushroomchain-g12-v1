package httpapi

import (
	"net/http"
	"strconv"
	"time"

	"github.com/VanceMichael/go-label-yanji-mushroomchain-g12-v1/internal/domain"
	"github.com/VanceMichael/go-label-yanji-mushroomchain-g12-v1/internal/middleware"
	"github.com/VanceMichael/go-label-yanji-mushroomchain-g12-v1/internal/pagination"
	"github.com/VanceMichael/go-label-yanji-mushroomchain-g12-v1/internal/repository"
	"github.com/VanceMichael/go-label-yanji-mushroomchain-g12-v1/internal/service"
)

func (a *API) registerBatch(w http.ResponseWriter, r *http.Request) {
	var body struct {
		FarmID         string    `json:"farm_id"`
		Code           string    `json:"code"`
		Species        string    `json:"species"`
		ProducedAt     time.Time `json:"produced_at"`
		ExpiresAt      time.Time `json:"expires_at"`
		Quantity       int64     `json:"quantity"`
		UnitPriceCents int64     `json:"unit_price_cents"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, r, err)
		return
	}
	batch, err := a.batches.Register(r.Context(), service.RegisterBatchInput{FarmID: body.FarmID, Code: body.Code, Species: body.Species, ProducedAt: body.ProducedAt, ExpiresAt: body.ExpiresAt, Quantity: body.Quantity, UnitPriceCents: body.UnitPriceCents, RequestID: middleware.RequestID(r.Context())})
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, batch)
}
func (a *API) getBatch(w http.ResponseWriter, r *http.Request) {
	batch, err := a.batches.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, batch)
}
func (a *API) listBatches(w http.ResponseWriter, r *http.Request) {
	page, err := pagination.Parse(r.URL.Query().Get("page"), r.URL.Query().Get("page_size"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	result, err := a.batches.List(r.Context(), repository.BatchFilter{FarmID: r.URL.Query().Get("farm_id"), Species: r.URL.Query().Get("species"), Status: domain.BatchStatus(r.URL.Query().Get("status")), Page: page})
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}
func (a *API) inspectBatch(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Decision        domain.InspectionDecision `json:"decision"`
		MoistureBP      int                       `json:"moisture_bp"`
		SampleCount     int                       `json:"sample_count"`
		Notes           string                    `json:"notes"`
		ExpectedVersion int64                     `json:"expected_version"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, r, err)
		return
	}
	batch, err := a.batches.Inspect(r.Context(), service.InspectBatchInput{BatchID: r.PathValue("id"), Decision: body.Decision, MoistureBP: body.MoistureBP, SampleCount: body.SampleCount, Notes: body.Notes, ExpectedVersion: body.ExpectedVersion, RequestID: middleware.RequestID(r.Context())})
	if err != nil {
		writeError(w, r, err)
		return
	}
	w.Header().Set("ETag", strconv.FormatInt(batch.Version, 10))
	writeJSON(w, http.StatusOK, batch)
}
