package repository_test

import (
	"context"
	"errors"
	"testing"

	"github.com/fastcheck/anonymus_bot/backend/internal/models"
	"github.com/fastcheck/anonymus_bot/backend/internal/repository"
)

func TestSwitchActiveSessionTx_Success(t *testing.T) {
	pool := setupTestDB(t)
	encryptor := testEncryptor(t)
	userRepo := repository.NewUserRepository(pool, encryptor)
	sessionRepo := repository.NewSessionRepository(pool)
	ctx := context.Background()

	user := &models.User{TelegramID: 111, LanguageCode: "ru"}
	if err := userRepo.Create(ctx, user); err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	session := &models.Session{Title: "Test", Status: models.SessionStatusActive, Language: "ru", ClientUserID: &user.ID}
	if err := sessionRepo.Create(ctx, session); err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	if err := userRepo.SwitchActiveSessionTx(ctx, user.ID, session.ID); err != nil {
		t.Fatalf("expected successful switch, got error: %v", err)
	}

	fetched, err := userRepo.GetByID(ctx, user.ID)
	if err != nil {
		t.Fatalf("failed to get user: %v", err)
	}

	if fetched.ActiveSessionID == nil || *fetched.ActiveSessionID != session.ID {
		t.Errorf("expected active_session_id %d, got %v", session.ID, fetched.ActiveSessionID)
	}
}

func TestSwitchActiveSessionTx_NotParticipant(t *testing.T) {
	pool := setupTestDB(t)
	encryptor := testEncryptor(t)
	userRepo := repository.NewUserRepository(pool, encryptor)
	sessionRepo := repository.NewSessionRepository(pool)
	ctx := context.Background()

	user := &models.User{TelegramID: 222, LanguageCode: "ru"}
	if err := userRepo.Create(ctx, user); err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	otherUser := &models.User{TelegramID: 333, LanguageCode: "ru"}
	if err := userRepo.Create(ctx, otherUser); err != nil {
		t.Fatalf("failed to create other user: %v", err)
	}

	// Сессия принадлежит otherUser, не user — попытка переключения должна быть отклонена.
	session := &models.Session{Title: "Not Mine", Status: models.SessionStatusActive, Language: "ru", ClientUserID: &otherUser.ID}
	if err := sessionRepo.Create(ctx, session); err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	err := userRepo.SwitchActiveSessionTx(ctx, user.ID, session.ID)
	if !errors.Is(err, repository.ErrNotParticipant) {
		t.Errorf("expected ErrNotParticipant, got %v", err)
	}

	fetched, err := userRepo.GetByID(ctx, user.ID)
	if err != nil {
		t.Fatalf("failed to get user: %v", err)
	}
	if fetched.ActiveSessionID != nil {
		t.Errorf("expected active_session_id to remain nil after rejected switch, got %v", fetched.ActiveSessionID)
	}
}
