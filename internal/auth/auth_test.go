package auth

import (
	"errors"
	"strings"
	"testing"
)

func TestPasswordHashAndVerify(t *testing.T) {
	password := "mushroom-production-password"
	encoded, err := HashPassword(password)
	if err != nil {
		t.Fatal(err)
	}
	if encoded == password {
		t.Fatal("password stored in clear text")
	}
	if !strings.HasPrefix(encoded, "$2") {
		t.Fatalf("unexpected encoding %q", encoded)
	}
	if err = VerifyPassword(encoded, password); err != nil {
		t.Fatalf("verify correct password: %v", err)
	}
	if err = VerifyPassword(encoded, "wrong-password-value"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("wrong password error=%v", err)
	}
}

func TestPasswordHashUsesUniqueSalt(t *testing.T) {
	first, err := HashPassword("mushroom-production-password")
	if err != nil {
		t.Fatal(err)
	}
	second, err := HashPassword("mushroom-production-password")
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("hashes reused a salt")
	}
}

func TestPasswordLengthValidation(t *testing.T) {
	for _, value := range []string{"", "short", strings.Repeat("x", 73)} {
		if _, err := HashPassword(value); err == nil {
			t.Fatalf("HashPassword accepted length %d", len(value))
		}
	}
	if _, err := HashPassword(strings.Repeat("x", 12)); err != nil {
		t.Fatalf("minimum password rejected: %v", err)
	}
	if _, err := HashPassword(strings.Repeat("x", 72)); err != nil {
		t.Fatalf("maximum password rejected: %v", err)
	}
}

func TestVerifyRejectsMalformedHashes(t *testing.T) {
	values := []string{"", "plain", "$2a$12$short", "$2b$01$invalidinvalidinvalidinvalidinvalidinvalidinvalidinvalid", "sha256i$120000$AA$BB"}
	for _, value := range values {
		if err := VerifyPassword(value, "mushroom-production-password"); !errors.Is(err, ErrInvalidCredentials) {
			t.Fatalf("VerifyPassword(%q) error=%v", value, err)
		}
	}
}

func TestTokensAreUniqueAndHashable(t *testing.T) {
	first, firstHash, err := NewToken()
	if err != nil {
		t.Fatal(err)
	}
	second, secondHash, err := NewToken()
	if err != nil {
		t.Fatal(err)
	}
	if first == second || firstHash == secondHash {
		t.Fatal("token collision")
	}
	if HashToken(first) != firstHash {
		t.Fatal("token hash mismatch")
	}
	if len(firstHash) != 64 {
		t.Fatalf("hash length=%d", len(firstHash))
	}
	if strings.Contains(first, "+") || strings.Contains(first, "/") {
		t.Fatalf("token is not URL safe: %q", first)
	}
}

func TestNewIDIncludesPrefix(t *testing.T) {
	first, err := NewID("batch")
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewID("batch")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(first, "batch_") {
		t.Fatalf("id=%q", first)
	}
	if first == second {
		t.Fatal("id collision")
	}
}
