package repository_test

import (
	"context"
	"testing"

	"github.com/fastcheck/anonymus_bot/backend/internal/models"
	"github.com/fastcheck/anonymus_bot/backend/internal/repository"
)

func TestMessageRepository_CreateAndListBySessionID(t *testing.T) {
	pool := setupTestDB(t)
	sessionRepo := repository.NewSessionRepository(pool)
	messageRepo := repository.NewMessageRepository(pool)
	ctx := context.Background()

	session := &models.Session{
		Title:    "Test Session",
		Status:   models.SessionStatusActive,
		Language: "ru",
	}
	if err := sessionRepo.Create(ctx, session); err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	content := "Hello, world!"
	message := &models.Message{
		SessionID:   session.ID,
		SenderRole:  models.SenderRoleClient,
		ContentType: models.ContentTypeText,
		Content:     &content,
	}

	if err := messageRepo.Create(ctx, message); err != nil {
		t.Fatalf("failed to create message: %v", err)
	}

	if message.ID == 0 {
		t.Fatal("expected message ID to be set after creation")
	}

	messages, err := messageRepo.ListBySessionID(ctx, session.ID)
	if err != nil {
		t.Fatalf("failed to list messages: %v", err)
	}

	if len(messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(messages))
	}

	if *messages[0].Content != content {
		t.Errorf("expected content %q, got %q", content, *messages[0].Content)
	}
}

func TestMessageRepository_ListBySessionID_OrderedByCreatedAt(t *testing.T) {
	pool := setupTestDB(t)
	sessionRepo := repository.NewSessionRepository(pool)
	messageRepo := repository.NewMessageRepository(pool)
	ctx := context.Background()

	session := &models.Session{
		Title:    "Test Session",
		Status:   models.SessionStatusActive,
		Language: "ru",
	}
	if err := sessionRepo.Create(ctx, session); err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	firstContent := "First message"
	secondContent := "Second message"

	msg1 := &models.Message{
		SessionID:   session.ID,
		SenderRole:  models.SenderRoleClient,
		ContentType: models.ContentTypeText,
		Content:     &firstContent,
	}
	msg2 := &models.Message{
		SessionID:   session.ID,
		SenderRole:  models.SenderRoleExecutor,
		ContentType: models.ContentTypeText,
		Content:     &secondContent,
	}

	if err := messageRepo.Create(ctx, msg1); err != nil {
		t.Fatalf("failed to create first message: %v", err)
	}
	if err := messageRepo.Create(ctx, msg2); err != nil {
		t.Fatalf("failed to create second message: %v", err)
	}

	messages, err := messageRepo.ListBySessionID(ctx, session.ID)
	if err != nil {
		t.Fatalf("failed to list messages: %v", err)
	}

	if len(messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(messages))
	}

	if *messages[0].Content != firstContent {
		t.Errorf("expected first message to be %q, got %q", firstContent, *messages[0].Content)
	}
	if *messages[1].Content != secondContent {
		t.Errorf("expected second message to be %q, got %q", secondContent, *messages[1].Content)
	}
}
