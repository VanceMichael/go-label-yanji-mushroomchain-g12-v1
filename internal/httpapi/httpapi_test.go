package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/VanceMichael/go-label-yanji-mushroomchain-g12-v1/internal/clock"
	"github.com/VanceMichael/go-label-yanji-mushroomchain-g12-v1/internal/domain"
	"github.com/VanceMichael/go-label-yanji-mushroomchain-g12-v1/internal/service"
	"github.com/VanceMichael/go-label-yanji-mushroomchain-g12-v1/internal/testkit"
)

type apiFixture struct {
	data    testkit.Fixture
	handler http.Handler
	auth    *service.AuthService
}

func newAPIFixture(t *testing.T) apiFixture {
	t.Helper()
	data := testkit.New(t)
	fixed := clock.NewFixed(data.Now)
	authService := service.NewAuth(data.Store, fixed, time.Hour)
	handler := New(authService, service.NewBatches(data.Store, data.Store, fixed), service.NewOrders(data.Store, data.Store, fixed), service.NewSettlements(data.Store, data.Store, fixed), data.Store, slog.New(slog.NewTextHandler(io.Discard, nil)))
	return apiFixture{data: data, handler: handler, auth: authService}
}

func (f apiFixture) request(t *testing.T, method, path, token string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var payload io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		payload = bytes.NewReader(encoded)
	}
	request := httptest.NewRequest(method, path, payload)
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	f.handler.ServeHTTP(recorder, request)
	return recorder
}

func (f apiFixture) login(t *testing.T, role domain.Role) string {
	t.Helper()
	recorder := f.request(t, http.MethodPost, "/v1/auth/login", "", map[string]any{"tenant_id": f.data.TenantID, "email": f.data.Users[role].Email, "password": f.data.Password})
	if recorder.Code != http.StatusOK {
		t.Fatalf("login status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var result struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Token == "" {
		t.Fatal("empty token")
	}
	return result.Token
}

func TestHealthAndReadiness(t *testing.T) {
	f := newAPIFixture(t)
	health := f.request(t, http.MethodGet, "/healthz", "", nil)
	if health.Code != http.StatusOK {
		t.Fatalf("health=%d %s", health.Code, health.Body.String())
	}
	ready := f.request(t, http.MethodGet, "/readyz", "", nil)
	if ready.Code != http.StatusOK {
		t.Fatalf("ready=%d %s", ready.Code, ready.Body.String())
	}
	if health.Header().Get("X-Request-ID") == "" || ready.Header().Get("X-Request-ID") == "" {
		t.Fatal("request id missing")
	}
}

func TestReadinessReportsDependencyFailure(t *testing.T) {
	data := testkit.New(t)
	fixed := clock.NewFixed(data.Now)
	authService := service.NewAuth(data.Store, fixed, time.Hour)
	handler := New(authService, service.NewBatches(data.Store, data.Store, fixed), service.NewOrders(data.Store, data.Store, fixed), service.NewSettlements(data.Store, data.Store, fixed), failingReadiness{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	request := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

type failingReadiness struct{}

func (failingReadiness) Ping(context.Context) error { return errors.New("database unavailable") }

func TestLoginRejectsMalformedAndIncorrectCredentials(t *testing.T) {
	f := newAPIFixture(t)
	request := httptest.NewRequest(http.MethodPost, "/v1/auth/login", bytes.NewBufferString(`{"tenant_id":"x","unknown":true}`))
	recorder := httptest.NewRecorder()
	f.handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("unknown field status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	wrong := f.request(t, http.MethodPost, "/v1/auth/login", "", map[string]any{"tenant_id": f.data.TenantID, "email": f.data.Users[domain.RoleOperator].Email, "password": "wrong-password-value"})
	if wrong.Code != http.StatusUnauthorized {
		t.Fatalf("wrong login status=%d body=%s", wrong.Code, wrong.Body.String())
	}
	assertErrorContract(t, wrong, "unauthorized")
}

func TestProtectedRouteRequiresBearerToken(t *testing.T) {
	f := newAPIFixture(t)
	for _, header := range []string{"", "Basic abc", "Bearer", "Bearer one two"} {
		request := httptest.NewRequest(http.MethodGet, "/v1/batches", nil)
		request.Header.Set("Authorization", header)
		recorder := httptest.NewRecorder()
		f.handler.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusUnauthorized {
			t.Fatalf("header %q status=%d", header, recorder.Code)
		}
		assertErrorContract(t, recorder, "unauthorized")
	}
}

func TestLogoutRevokesToken(t *testing.T) {
	f := newAPIFixture(t)
	token := f.login(t, domain.RoleOperator)
	logout := f.request(t, http.MethodPost, "/v1/auth/logout", token, nil)
	if logout.Code != http.StatusNoContent {
		t.Fatalf("logout=%d %s", logout.Code, logout.Body.String())
	}
	after := f.request(t, http.MethodGet, "/v1/batches", token, nil)
	if after.Code != http.StatusUnauthorized {
		t.Fatalf("revoked token status=%d body=%s", after.Code, after.Body.String())
	}
	assertErrorContract(t, after, "session_expired")
}

func TestBatchRegistrationAndInspectionOverHTTP(t *testing.T) {
	f := newAPIFixture(t)
	operator := f.login(t, domain.RoleOperator)
	register := f.request(t, http.MethodPost, "/v1/batches", operator, map[string]any{"farm_id": f.data.Farm.ID, "code": "HTTP-BATCH", "species": "oyster", "produced_at": f.data.Now.Add(-time.Hour), "expires_at": f.data.Now.Add(30 * 24 * time.Hour), "quantity": 200, "unit_price_cents": 350})
	if register.Code != http.StatusCreated {
		t.Fatalf("register=%d %s", register.Code, register.Body.String())
	}
	var batch domain.SubstrateBatch
	if err := json.Unmarshal(register.Body.Bytes(), &batch); err != nil {
		t.Fatal(err)
	}
	if batch.Status != domain.BatchRegistered {
		t.Fatalf("batch=%+v", batch)
	}
	inspector := f.login(t, domain.RoleInspector)
	inspection := f.request(t, http.MethodPost, "/v1/batches/"+batch.ID+"/inspections", inspector, map[string]any{"decision": "approved", "moisture_bp": 6200, "sample_count": 10, "notes": "accepted", "expected_version": 1})
	if inspection.Code != http.StatusOK {
		t.Fatalf("inspect=%d %s", inspection.Code, inspection.Body.String())
	}
	if err := json.Unmarshal(inspection.Body.Bytes(), &batch); err != nil {
		t.Fatal(err)
	}
	if batch.Status != domain.BatchReleased || batch.Version != 3 {
		t.Fatalf("released=%+v", batch)
	}
	if inspection.Header().Get("ETag") != "3" {
		t.Fatalf("etag=%q", inspection.Header().Get("ETag"))
	}
}

func TestRoleViolationReturnsForbiddenWithoutMutation(t *testing.T) {
	f := newAPIFixture(t)
	operator := f.login(t, domain.RoleOperator)
	register := f.request(t, http.MethodPost, "/v1/batches", operator, map[string]any{"farm_id": f.data.Farm.ID, "code": "ROLE-BATCH", "species": "oyster", "produced_at": f.data.Now.Add(-time.Hour), "expires_at": f.data.Now.Add(24 * time.Hour), "quantity": 20, "unit_price_cents": 300})
	var batch domain.SubstrateBatch
	_ = json.Unmarshal(register.Body.Bytes(), &batch)
	farmer := f.login(t, domain.RoleFarmer)
	inspection := f.request(t, http.MethodPost, "/v1/batches/"+batch.ID+"/inspections", farmer, map[string]any{"decision": "approved", "moisture_bp": 6200, "sample_count": 5, "expected_version": 1})
	if inspection.Code != http.StatusForbidden {
		t.Fatalf("status=%d body=%s", inspection.Code, inspection.Body.String())
	}
	assertErrorContract(t, inspection, "forbidden")
	stored, err := f.data.Store.GetBatch(context.Background(), f.data.TenantID, batch.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != domain.BatchRegistered || stored.Version != 1 {
		t.Fatalf("unauthorized mutation=%+v", stored)
	}
}

func TestOrderValidationAndPaginationErrors(t *testing.T) {
	f := newAPIFixture(t)
	dispatcher := f.login(t, domain.RoleDispatcher)
	invalid := f.request(t, http.MethodPost, "/v1/orders", dispatcher, map[string]any{"buyer_name": "Buyer", "delivery_region": "Yinchuan", "due_at": f.data.Now.Add(time.Hour), "lines": []any{}})
	if invalid.Code != http.StatusBadRequest {
		t.Fatalf("invalid order=%d %s", invalid.Code, invalid.Body.String())
	}
	page := f.request(t, http.MethodGet, "/v1/orders?page=0&page_size=500", dispatcher, nil)
	if page.Code != http.StatusBadRequest {
		t.Fatalf("invalid page=%d %s", page.Code, page.Body.String())
	}
	assertErrorContract(t, page, "invalid_request")
}

func TestRequestIDIsPreservedInErrors(t *testing.T) {
	f := newAPIFixture(t)
	request := httptest.NewRequest(http.MethodGet, "/v1/batches", nil)
	request.Header.Set("X-Request-ID", "client-request-123")
	recorder := httptest.NewRecorder()
	f.handler.ServeHTTP(recorder, request)
	if recorder.Header().Get("X-Request-ID") != "client-request-123" {
		t.Fatalf("header=%q", recorder.Header().Get("X-Request-ID"))
	}
	var body errorBody
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Error.RequestID != "client-request-123" {
		t.Fatalf("body=%+v", body)
	}
}

func assertErrorContract(t *testing.T, recorder *httptest.ResponseRecorder, code string) {
	t.Helper()
	var body errorBody
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode error body: %v: %s", err, recorder.Body.String())
	}
	if body.Error.Code != code {
		t.Fatalf("code=%q want %q", body.Error.Code, code)
	}
	if body.Error.Message == "" || body.Error.RequestID == "" {
		t.Fatalf("incomplete error=%+v", body.Error)
	}
}
