package httpapi

import (
	"net/http"

	"github.com/VanceMichael/go-label-yanji-mushroomchain-g12-v1/internal/service"
)

func (a *API) login(w http.ResponseWriter, r *http.Request) {
	var body struct {
		TenantID string `json:"tenant_id"`
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, r, err)
		return
	}
	result, err := a.auth.Login(r.Context(), service.LoginInput{TenantID: body.TenantID, Email: body.Email, Password: body.Password})
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}
func (a *API) logout(w http.ResponseWriter, r *http.Request) {
	if err := a.auth.Logout(r.Context()); err != nil {
		writeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
