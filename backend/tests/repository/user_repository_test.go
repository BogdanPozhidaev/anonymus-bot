package repository_test

import (
	"context"
	"testing"

	"github.com/fastcheck/anonymus_bot/backend/internal/crypto"
	"github.com/fastcheck/anonymus_bot/backend/internal/models"
	"github.com/fastcheck/anonymus_bot/backend/internal/repository"
)

func testEncryptor(t *testing.T) *crypto.Encryptor {
	t.Helper()
	key := make([]byte, 32)
	enc, err := crypto.NewEncryptor(key)
	if err != nil {
		t.Fatalf("failed to create test encryptor: %v", err)
	}
	return enc
}

func TestUserRepository_CreateAndGetByID(t *testing.T) {
	pool := setupTestDB(t)
	encryptor := testEncryptor(t)
	repo := repository.NewUserRepository(pool, encryptor)
	ctx := context.Background()

	username := "john_doe"
	firstName := "John"

	user := &models.User{
		TelegramID:   123456789,
		Username:     &username,
		FirstName:    &firstName,
		LanguageCode: "en",
	}

	if err := repo.Create(ctx, user); err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	if user.ID == 0 {
		t.Fatal("expected user ID to be set after creation")
	}

	fetched, err := repo.GetByID(ctx, user.ID)
	if err != nil {
		t.Fatalf("failed to get user by id: %v", err)
	}

	if fetched.TelegramID != user.TelegramID {
		t.Errorf("expected telegram_id %d, got %d", user.TelegramID, fetched.TelegramID)
	}
	if *fetched.Username != username {
		t.Errorf("expected username %q, got %q", username, *fetched.Username)
	}
	if *fetched.FirstName != firstName {
		t.Errorf("expected first_name %q, got %q", firstName, *fetched.FirstName)
	}
}

func TestUserRepository_GetByTelegramID(t *testing.T) {
	pool := setupTestDB(t)
	encryptor := testEncryptor(t)
	repo := repository.NewUserRepository(pool, encryptor)
	ctx := context.Background()

	user := &models.User{
		TelegramID:   987654321,
		LanguageCode: "ru",
	}

	if err := repo.Create(ctx, user); err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	fetched, err := repo.GetByTelegramID(ctx, 987654321)
	if err != nil {
		t.Fatalf("failed to get user by telegram_id: %v", err)
	}

	if fetched.ID != user.ID {
		t.Errorf("expected id %d, got %d", user.ID, fetched.ID)
	}
}

func TestUserRepository_GetByID_NotFound(t *testing.T) {
	pool := setupTestDB(t)
	encryptor := testEncryptor(t)
	repo := repository.NewUserRepository(pool, encryptor)
	ctx := context.Background()

	_, err := repo.GetByID(ctx, 999999)
	if err != repository.ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestUserRepository_Update(t *testing.T) {
	pool := setupTestDB(t)
	encryptor := testEncryptor(t)
	repo := repository.NewUserRepository(pool, encryptor)
	ctx := context.Background()

	user := &models.User{
		TelegramID:   111222333,
		LanguageCode: "ru",
	}
	if err := repo.Create(ctx, user); err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	newUsername := "updated_name"
	user.Username = &newUsername
	user.LanguageCode = "en"

	if err := repo.Update(ctx, user); err != nil {
		t.Fatalf("failed to update user: %v", err)
	}

	fetched, err := repo.GetByID(ctx, user.ID)
	if err != nil {
		t.Fatalf("failed to get updated user: %v", err)
	}

	if *fetched.Username != newUsername {
		t.Errorf("expected username %q, got %q", newUsername, *fetched.Username)
	}
	if fetched.LanguageCode != "en" {
		t.Errorf("expected language_code %q, got %q", "en", fetched.LanguageCode)
	}
}
