package api

import (
	"testing"

	"github.com/fastcheck/anonymus_bot/backend/internal/models"
)

func TestDetermineRoleAndRecipient_SenderIsClient(t *testing.T) {
	clientID := int64(10)
	executorID := int64(20)
	session := &models.Session{
		ClientUserID:   &clientID,
		ExecutorUserID: &executorID,
	}

	role, recipientID, label := determineRoleAndRecipient(session, 10)

	if role != models.SenderRoleClient {
		t.Errorf("expected role client, got %s", role)
	}
	if recipientID == nil || *recipientID != 20 {
		t.Errorf("expected recipient 20, got %v", recipientID)
	}
	if label != "Клиент:" {
		t.Errorf("expected label 'Клиент:', got %s", label)
	}
}

func TestDetermineRoleAndRecipient_SenderIsExecutor(t *testing.T) {
	clientID := int64(10)
	executorID := int64(20)
	session := &models.Session{
		ClientUserID:   &clientID,
		ExecutorUserID: &executorID,
	}

	role, recipientID, label := determineRoleAndRecipient(session, 20)

	if role != models.SenderRoleExecutor {
		t.Errorf("expected role executor, got %s", role)
	}
	if recipientID == nil || *recipientID != 10 {
		t.Errorf("expected recipient 10, got %v", recipientID)
	}
	if label != "Менеджер:" {
		t.Errorf("expected label 'Менеджер:', got %s", label)
	}
}

func TestDetermineRoleAndRecipient_CounterpartyNotBound(t *testing.T) {
	clientID := int64(10)
	session := &models.Session{
		ClientUserID:   &clientID,
		ExecutorUserID: nil, // исполнитель ещё не подключился
	}

	role, recipientID, label := determineRoleAndRecipient(session, 10)

	if role != models.SenderRoleClient {
		t.Errorf("expected role client, got %s", role)
	}
	if recipientID != nil {
		t.Errorf("expected nil recipient, got %v", recipientID)
	}
	if label != "Клиент:" {
		t.Errorf("expected label 'Клиент:', got %s", label)
	}
}

func TestDetermineRoleAndRecipient_SenderNotInSession(t *testing.T) {
	clientID := int64(10)
	executorID := int64(20)
	session := &models.Session{
		ClientUserID:   &clientID,
		ExecutorUserID: &executorID,
	}

	role, recipientID, label := determineRoleAndRecipient(session, 999)

	if role != "" {
		t.Errorf("expected empty role, got %s", role)
	}
	if recipientID != nil {
		t.Errorf("expected nil recipient, got %v", recipientID)
	}
	if label != "" {
		t.Errorf("expected empty label, got %s", label)
	}
}
