package auth_test

import (
	"testing"

	"github.com/fastcheck/anonymus_bot/backend/internal/auth"
)

func TestHashAndVerifyPassword(t *testing.T) {
	password := "SuperSecret123!"

	hash, err := auth.HashPassword(password)
	if err != nil {
		t.Fatalf("failed to hash password: %v", err)
	}

	if hash == password {
		t.Error("hash should not equal plaintext password")
	}

	if !auth.VerifyPassword(hash, password) {
		t.Error("expected password to verify successfully")
	}

	if auth.VerifyPassword(hash, "WrongPassword") {
		t.Error("expected wrong password to fail verification")
	}
}
