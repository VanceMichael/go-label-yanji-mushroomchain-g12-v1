package httpapi

import (
	"net/http"
	"testing"

	"github.com/VanceMichael/go-label-yanji-mushroomchain-g12-v1/internal/domain"
)

func TestAuthenticationDependencyFailurePreservesServerError(t *testing.T) {
	f := newAPIFixture(t)
	token := f.login(t, domain.RoleOperator)

	healthy := f.request(t, http.MethodGet, "/v1/batches", token, nil)
	if healthy.Code != http.StatusOK {
		t.Fatalf("valid token before dependency failure status=%d body=%s", healthy.Code, healthy.Body.String())
	}
	if err := f.data.Store.Close(); err != nil {
		t.Fatalf("close backing store: %v", err)
	}

	failed := f.request(t, http.MethodGet, "/v1/batches", token, nil)
	if failed.Code != http.StatusInternalServerError {
		t.Fatalf("dependency failure status=%d body=%s", failed.Code, failed.Body.String())
	}
	assertErrorContract(t, failed, "internal")
	if failed.Header().Get("X-Request-ID") == "" {
		t.Fatal("dependency error lost request id")
	}
}
