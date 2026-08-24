package service

import (
	"context"
	"errors"

	"github.com/VanceMichael/go-label-yanji-mushroomchain-g12-v1/internal/domain"
)

type identityKey struct{}

type Identity struct {
	UserID    string
	TenantID  string
	FarmID    string
	SessionID string
	Role      domain.Role
}

func WithIdentity(ctx context.Context, identity Identity) context.Context {
	return context.WithValue(ctx, identityKey{}, identity)
}

func IdentityFrom(ctx context.Context) (Identity, error) {
	identity, ok := ctx.Value(identityKey{}).(Identity)
	if !ok || identity.UserID == "" || identity.TenantID == "" || !identity.Role.Valid() {
		return Identity{}, domain.ErrUnauthorized
	}
	return identity, nil
}

func RequireRoles(ctx context.Context, roles ...domain.Role) (Identity, error) {
	identity, err := IdentityFrom(ctx)
	if err != nil {
		return Identity{}, err
	}
	for _, role := range roles {
		if identity.Role == role {
			return identity, nil
		}
	}
	return Identity{}, errors.Join(domain.ErrForbidden, errors.New("role is not permitted"))
}
