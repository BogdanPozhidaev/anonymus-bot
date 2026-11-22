package repository_test

import (
	"bytes"
	"encoding/base64"
	"strings"
	"testing"

	"github.com/fastcheck/anonymus_bot/backend/internal/config"
	"github.com/fastcheck/anonymus_bot/backend/internal/crypto"
	"github.com/fastcheck/anonymus_bot/backend/internal/models"
	"github.com/fastcheck/anonymus_bot/backend/internal/repository"
)

func TestEncryptionKeyRotation(t *testing.T) {
	ctx := t.Context()
	pool := setupTestDB(t)
	oldKey := bytes.Repeat([]byte{1}, 32)
	t.Setenv("PII_ENCRYPTION_KEY", base64.StdEncoding.EncodeToString(oldKey))
	t.Setenv("PII_ENCRYPTION_KEYS", "2:"+base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{2}, 32)))
	t.Setenv("PII_ENCRYPTION_ACTIVE_VERSION", "2")
	keys, err := config.LoadEncryption()
	if err != nil {
		t.Fatal(err)
	}
	oldPII, err := crypto.NewEncryptor(oldKey)
	if err != nil {
		t.Fatal(err)
	}
	oldTOTP, err := crypto.NewEncryptor(crypto.DeriveKey(oldKey, "operator TOTP encryption v1"))
	if err != nil {
		t.Fatal(err)
	}
	const secret = "JBSWY3DPEHPK3PXP"
	oldSecret, err := oldTOTP.EncryptTOTP(secret)
	if err != nil {
		t.Fatal(err)
	}
	firstName := "fixture participant"
	user := &models.User{TelegramID: 9001, LanguageCode: "en", FirstName: &firstName}
	if err := repository.NewUserRepository(pool, oldPII).Create(ctx, user); err != nil {
		t.Fatal(err)
	}
	operatorRepo := repository.NewOperatorRepository(pool)
	var operatorIDs []int64
	for login, material := range map[string]string{"encrypted": oldSecret, "plaintext": secret} {
		operator := &models.Operator{Name: login, Login: login, Role: "admin", Status: "active", Language: "en"}
		if err := operatorRepo.Create(ctx, operator); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, "UPDATE operators SET totp_secret=$1 WHERE id=$2", material, operator.ID); err != nil {
			t.Fatal(err)
		}
		operatorIDs = append(operatorIDs, operator.ID)
	}
	storedName := func() string {
		t.Helper()
		var stored string
		if err := pool.QueryRow(ctx, "SELECT first_name FROM users WHERE id=$1", user.ID).Scan(&stored); err != nil {
			t.Fatal(err)
		}
		return stored
	}
	original := storedName()
	rotate := func(apply bool) (repository.KeyRotationResult, error) {
		return repository.RotateEncryptionKeys(ctx, pool, keys.PII, keys.TOTP, apply)
	}
	check, err := rotate(false)
	if err != nil || check.Users != 1 || check.Operators != 2 || storedName() != original {
		t.Fatalf("unexpected dry run: %+v, %v", check, err)
	}
	lock, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = lock.Rollback(ctx) }()
	if _, err := lock.Exec(ctx, "LOCK TABLE users IN ROW EXCLUSIVE MODE"); err != nil {
		t.Fatal(err)
	}
	if _, err := rotate(true); err == nil {
		t.Fatal("rotation accepted a conflicting writer")
	}
	if err := lock.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "UPDATE operators SET totp_secret='totp:v1:broken@fixture' WHERE id=$1", operatorIDs[0]); err != nil {
		t.Fatal(err)
	}
	if _, err := rotate(true); err == nil || strings.Contains(err.Error(), "broken@fixture") || storedName() != original {
		t.Fatal("failed rotation should roll back and keep ciphertext out of errors")
	}
	if _, err := pool.Exec(ctx, "UPDATE operators SET totp_secret=$1 WHERE id=$2", oldSecret, operatorIDs[0]); err != nil {
		t.Fatal(err)
	}
	if applied, err := rotate(true); err != nil || applied != check {
		t.Fatalf("unexpected apply result: %+v, %v", applied, err)
	}
	t.Setenv("PII_ENCRYPTION_KEY", "")
	keys, err = config.LoadEncryption()
	if err != nil {
		t.Fatal(err)
	}
	if got, err := repository.NewUserRepository(pool, keys.PII).GetByID(ctx, user.ID); err != nil || got.FirstName == nil || *got.FirstName != firstName || got.Username != nil {
		t.Fatalf("rotated PII could not be read: %v", err)
	}
	for _, id := range operatorIDs {
		op, err := operatorRepo.GetByID(ctx, id)
		if err != nil || op.TOTPSecret == nil || !strings.HasPrefix(*op.TOTPSecret, "totp:v1:aesgcm:2:") {
			t.Fatalf("TOTP was not rotated: %v", err)
		}
		if got, legacy, err := keys.TOTP.DecryptTOTP(*op.TOTPSecret); err != nil || legacy || got != secret || op.AuthVersion != 1 {
			t.Fatalf("TOTP or operator authentication state changed: %v", err)
		}
	}
	if applied, err := rotate(true); err != nil || applied != (repository.KeyRotationResult{}) {
		t.Fatalf("rerun after retiring the old key: %+v, %v", applied, err)
	}
}
