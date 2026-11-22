package auth_test

import (
	"strings"
	"testing"

	"github.com/fastcheck/anonymus_bot/backend/internal/auth"
)

func TestPasswordPolicy(t *testing.T) {
	for _, fixture := range []struct {
		password string
		valid    bool
	}{
		{"short", false},
		{strings.Repeat(" ", 12), false},
		{"fixture\npassword", false},
		{strings.Repeat("a", 12), true},
		{strings.Repeat("🔑", 18), true},
		{strings.Repeat("🔑", 19), false},
	} {
		if valid := auth.ValidateNewPassword(fixture.password) == nil; valid != fixture.valid {
			t.Errorf("password policy returned %t for %d bytes, want %t", valid, len(fixture.password), fixture.valid)
		}
	}
}

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
