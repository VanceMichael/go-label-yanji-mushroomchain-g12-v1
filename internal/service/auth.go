package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/VanceMichael/go-label-yanji-mushroomchain-g12-v1/internal/auth"
	"github.com/VanceMichael/go-label-yanji-mushroomchain-g12-v1/internal/clock"
	"github.com/VanceMichael/go-label-yanji-mushroomchain-g12-v1/internal/domain"
	"github.com/VanceMichael/go-label-yanji-mushroomchain-g12-v1/internal/repository"
)

type AuthService struct {
	repo  repository.Tx
	clock clock.Clock
	ttl   time.Duration
}

type LoginInput struct{ TenantID, Email, Password string }
type LoginResult struct {
	Token     string      `json:"token"`
	ExpiresAt time.Time   `json:"expires_at"`
	User      domain.User `json:"user"`
}

func NewAuth(repo repository.Tx, c clock.Clock, ttl time.Duration) *AuthService {
	return &AuthService{repo: repo, clock: c, ttl: ttl}
}

func (s *AuthService) Login(ctx context.Context, input LoginInput) (LoginResult, error) {
	if strings.TrimSpace(input.TenantID) == "" || strings.TrimSpace(input.Email) == "" || input.Password == "" {
		return LoginResult{}, domain.ErrInvalid
	}
	user, err := s.repo.FindUserByEmail(ctx, input.TenantID, input.Email)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return LoginResult{}, domain.ErrUnauthorized
		}
		return LoginResult{}, err
	}
	if !user.Active || auth.VerifyPassword(user.PasswordHash, input.Password) != nil {
		return LoginResult{}, domain.ErrUnauthorized
	}
	token, hash, err := auth.NewToken()
	if err != nil {
		return LoginResult{}, err
	}
	id, err := auth.NewID("ses")
	if err != nil {
		return LoginResult{}, err
	}
	now := s.clock.Now()
	session := domain.Session{ID: id, UserID: user.ID, TokenHash: hash, ExpiresAt: now.Add(s.ttl), CreatedAt: now}
	if err := s.repo.CreateSession(ctx, session); err != nil {
		return LoginResult{}, err
	}
	return LoginResult{Token: token, ExpiresAt: session.ExpiresAt, User: user}, nil
}

func (s *AuthService) Authenticate(ctx context.Context, token string) (Identity, error) {
	if strings.TrimSpace(token) == "" {
		return Identity{}, domain.ErrUnauthorized
	}
	session, user, err := s.repo.FindSessionByTokenHash(ctx, auth.HashToken(token))
	if err != nil {
		return Identity{}, domain.ErrUnauthorized
	}
	if !session.ActiveAt(s.clock.Now()) || !user.Active {
		return Identity{}, domain.ErrExpired
	}
	return Identity{UserID: user.ID, TenantID: user.TenantID, FarmID: user.FarmID, SessionID: session.ID, Role: user.Role}, nil
}

func (s *AuthService) Logout(ctx context.Context) error {
	identity, err := IdentityFrom(ctx)
	if err != nil {
		return err
	}
	return s.repo.RevokeSession(ctx, identity.UserID, identity.SessionID, s.clock.Now())
}
