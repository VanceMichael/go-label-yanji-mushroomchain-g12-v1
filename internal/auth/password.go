package auth

import (
	"errors"

	"golang.org/x/crypto/bcrypt"
)

const passwordCost = bcrypt.DefaultCost

var ErrInvalidCredentials = errors.New("invalid credentials")

func HashPassword(password string) (string, error) {
	if len(password) < 12 || len(password) > 72 {
		return "", errors.New("password must contain 12 to 72 bytes")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), passwordCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

func VerifyPassword(encoded, password string) error {
	if err := bcrypt.CompareHashAndPassword([]byte(encoded), []byte(password)); err != nil {
		return ErrInvalidCredentials
	}
	return nil
}
