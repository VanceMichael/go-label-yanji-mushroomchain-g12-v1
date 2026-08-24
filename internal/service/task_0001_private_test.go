package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/VanceMichael/go-label-yanji-mushroomchain-g12-v1/internal/auth"
	"github.com/VanceMichael/go-label-yanji-mushroomchain-g12-v1/internal/clock"
	"github.com/VanceMichael/go-label-yanji-mushroomchain-g12-v1/internal/domain"
	"github.com/VanceMichael/go-label-yanji-mushroomchain-g12-v1/internal/testkit"
)

func TestLogoutOnlyRevokesCallingSession(t *testing.T) {
	data := testkit.New(t)
	authService := NewAuth(data.Store, clock.NewFixed(data.Now), time.Hour)
	login := LoginInput{
		TenantID: data.TenantID,
		Email:    data.Users[domain.RoleOperator].Email,
		Password: data.Password,
	}

	terminalA, err := authService.Login(context.Background(), login)
	if err != nil {
		t.Fatalf("login terminal A: %v", err)
	}
	terminalB, err := authService.Login(context.Background(), login)
	if err != nil {
		t.Fatalf("login terminal B: %v", err)
	}
	identityA, err := authService.Authenticate(context.Background(), terminalA.Token)
	if err != nil {
		t.Fatalf("authenticate terminal A: %v", err)
	}
	identityB, err := authService.Authenticate(context.Background(), terminalB.Token)
	if err != nil {
		t.Fatalf("authenticate terminal B: %v", err)
	}
	if identityA.SessionID == identityB.SessionID {
		t.Fatalf("independent logins share session %q", identityA.SessionID)
	}

	if err = authService.Logout(WithIdentity(context.Background(), identityA)); err != nil {
		t.Fatalf("logout terminal A: %v", err)
	}
	if _, err = authService.Authenticate(context.Background(), terminalA.Token); !errors.Is(err, domain.ErrExpired) {
		t.Fatalf("terminal A remains active after logout: %v", err)
	}
	if got, err := authService.Authenticate(context.Background(), terminalB.Token); err != nil || got.SessionID != identityB.SessionID {
		t.Fatalf("terminal B session changed after terminal A logout: identity=%+v err=%v", got, err)
	}

	storedA, _, err := data.Store.FindSessionByTokenHash(context.Background(), auth.HashToken(terminalA.Token))
	if err != nil {
		t.Fatalf("read terminal A session: %v", err)
	}
	storedB, _, err := data.Store.FindSessionByTokenHash(context.Background(), auth.HashToken(terminalB.Token))
	if err != nil {
		t.Fatalf("read terminal B session: %v", err)
	}
	if storedA.RevokedAt == nil || storedB.RevokedAt != nil {
		t.Fatalf("revocation scope terminalA=%v terminalB=%v", storedA.RevokedAt, storedB.RevokedAt)
	}

	if err = authService.Logout(WithIdentity(context.Background(), identityB)); err != nil {
		t.Fatalf("logout terminal B: %v", err)
	}
	if _, err = authService.Authenticate(context.Background(), terminalB.Token); !errors.Is(err, domain.ErrExpired) {
		t.Fatalf("terminal B remains active after its own logout: %v", err)
	}
}
