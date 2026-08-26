package middleware

import (
	"net/http"
	"strings"

	"github.com/VanceMichael/go-label-yanji-mushroomchain-g12-v1/internal/domain"
	"github.com/VanceMichael/go-label-yanji-mushroomchain-g12-v1/internal/service"
)

type Authenticator interface {
	Authenticate(ctx interface{ Done() <-chan struct{} }, token string) (service.Identity, error)
}

func Authenticate(auth *service.AuthService, onError func(http.ResponseWriter, *http.Request, error), next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := strings.TrimSpace(r.Header.Get("Authorization"))
		parts := strings.Fields(header)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			onError(w, r, domain.ErrUnauthorized)
			return
		}
		identity, err := auth.Authenticate(r.Context(), parts[1])
		if err != nil {
			onError(w, r, err)
			return
		}
		next.ServeHTTP(w, r.WithContext(service.WithIdentity(r.Context(), identity)))
	})
}
