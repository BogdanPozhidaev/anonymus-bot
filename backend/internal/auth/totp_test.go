package auth_test

import (
	"testing"
	"time"

	"github.com/pquerna/otp/totp"

	"github.com/fastcheck/anonymus_bot/backend/internal/auth"
)

func TestGenerateAndVerifyTOTP(t *testing.T) {
	secret, url, err := auth.GenerateTOTPSecret("test_operator", "AnonymusBot")
	if err != nil {
		t.Fatalf("failed to generate totp secret: %v", err)
	}

	if secret == "" {
		t.Fatal("expected non-empty secret")
	}
	if url == "" {
		t.Fatal("expected non-empty otpauth url")
	}

	code, err := totp.GenerateCode(secret, time.Now())
	if err != nil {
		t.Fatalf("failed to generate code for testing: %v", err)
	}

	if !auth.VerifyTOTPCode(secret, code) {
		t.Error("expected generated code to verify successfully")
	}

	if auth.VerifyTOTPCode(secret, "000000") {
		t.Error("expected arbitrary wrong code to fail verification (extremely unlikely to collide)")
	}
}
