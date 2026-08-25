package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/VanceMichael/go-label-yanji-mushroomchain-g12-v1/internal/domain"
	"github.com/VanceMichael/go-label-yanji-mushroomchain-g12-v1/internal/middleware"
	"github.com/VanceMichael/go-label-yanji-mushroomchain-g12-v1/internal/pagination"
)

type errorBody struct {
	Error errorDetail `json:"error"`
}
type errorDetail struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id"`
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	status, code, message := http.StatusInternalServerError, "internal", "internal server error"
	switch {
	case errors.Is(err, domain.ErrUnauthorized):
		status, code, message = http.StatusUnauthorized, "unauthorized", "authentication required"
	case errors.Is(err, domain.ErrExpired):
		status, code, message = http.StatusUnauthorized, "session_expired", "session expired or revoked"
	case errors.Is(err, domain.ErrForbidden):
		status, code, message = http.StatusForbidden, "forbidden", "operation is not permitted"
	case errors.Is(err, domain.ErrNotFound):
		status, code, message = http.StatusNotFound, "not_found", "resource was not found"
	case errors.Is(err, domain.ErrInvalid), errors.Is(err, pagination.ErrInvalidPage):
		status, code, message = http.StatusBadRequest, "invalid_request", err.Error()
	case errors.Is(err, domain.ErrState):
		status, code, message = http.StatusConflict, "invalid_state", err.Error()
	case errors.Is(err, domain.ErrVersionConflict):
		status, code, message = http.StatusConflict, "version_conflict", "resource changed; reload and retry"
	case errors.Is(err, domain.ErrCapacity):
		status, code, message = http.StatusConflict, "capacity_unavailable", "released inventory cannot satisfy the order"
	case errors.Is(err, domain.ErrConflict):
		status, code, message = http.StatusConflict, "conflict", err.Error()
	}
	writeJSON(w, status, errorBody{Error: errorDetail{Code: code, Message: message, RequestID: middleware.RequestID(r.Context())}})
}

func decodeJSON(r *http.Request, target any) error {
	decoder := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return domain.FieldError{Field: "body", Message: err.Error()}
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return domain.FieldError{Field: "body", Message: "must contain one JSON value"}
	}
	return nil
}
