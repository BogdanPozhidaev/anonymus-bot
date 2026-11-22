package models

import "testing"

func TestParticipantRole_SenderIsClient(t *testing.T) {
	clientID := int64(10)
	executorID := int64(20)
	session := &Session{
		ClientUserID:   &clientID,
		ExecutorUserID: &executorID,
	}

	role, recipientID := session.ParticipantRole(10)

	if role != SenderRoleClient {
		t.Errorf("expected role client, got %s", role)
	}
	if recipientID == nil || *recipientID != 20 {
		t.Errorf("expected recipient 20, got %v", recipientID)
	}
}

func TestParticipantRole_SenderIsExecutor(t *testing.T) {
	clientID := int64(10)
	executorID := int64(20)
	session := &Session{
		ClientUserID:   &clientID,
		ExecutorUserID: &executorID,
	}

	role, recipientID := session.ParticipantRole(20)

	if role != SenderRoleExecutor {
		t.Errorf("expected role executor, got %s", role)
	}
	if recipientID == nil || *recipientID != 10 {
		t.Errorf("expected recipient 10, got %v", recipientID)
	}
}

func TestParticipantRole_CounterpartyNotBound(t *testing.T) {
	clientID := int64(10)
	session := &Session{
		ClientUserID:   &clientID,
		ExecutorUserID: nil, // The other participant has not joined.
	}

	role, recipientID := session.ParticipantRole(10)

	if role != SenderRoleClient {
		t.Errorf("expected role client, got %s", role)
	}
	if recipientID != nil {
		t.Errorf("expected nil recipient, got %v", recipientID)
	}
}

func TestParticipantRole_SenderNotInSession(t *testing.T) {
	clientID := int64(10)
	executorID := int64(20)
	session := &Session{
		ClientUserID:   &clientID,
		ExecutorUserID: &executorID,
	}

	role, recipientID := session.ParticipantRole(999)

	if role != "" {
		t.Errorf("expected empty role, got %s", role)
	}
	if recipientID != nil {
		t.Errorf("expected nil recipient, got %v", recipientID)
	}
}
