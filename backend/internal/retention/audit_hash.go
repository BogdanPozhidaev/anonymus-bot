package retention

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/fastcheck/anonymus_bot/backend/internal/repository"
	"github.com/jackc/pgx/v5"
)

// piiFieldsInPayload lists the top-level audit fields that retention hashes.
var piiFieldsInPayload = []string{
	"username", "first_name", "telegram_id", "email", "phone",
}

type AuditLogHasher struct {
	key []byte
}

func NewAuditLogHasher(key []byte) *AuditLogHasher {
	return &AuditLogHasher{key: key}
}

// hashPIIInPayload replaces selected non-null values with salted HMAC digests.
func (h *AuditLogHasher) hashPIIInPayload(payload []byte) ([]byte, bool, error) {
	if len(payload) == 0 {
		return payload, false, nil
	}

	var data map[string]json.RawMessage
	if err := json.Unmarshal(payload, &data); err != nil {
		// Keep payloads that cannot be decoded as JSON objects.
		return payload, false, nil
	}

	changed := false
	for _, field := range piiFieldsInPayload {
		value, exists := data[field]
		if !exists || string(value) == "null" {
			continue
		}
		var input string
		if err := json.Unmarshal(value, &input); err != nil {
			input = string(value)
		} else if isHMAC(input) {
			continue
		}
		hashed, err := h.hashValue(field, input)
		if err != nil {
			return nil, false, err
		}
		data[field], _ = json.Marshal(hashed)
		changed = true
	}

	if !changed {
		return payload, false, nil
	}

	newPayload, err := json.Marshal(data)
	if err != nil {
		return nil, false, fmt.Errorf("failed to marshal hashed payload: %w", err)
	}

	return newPayload, true, nil
}

const auditHashPrefix = "hmac-sha256:v1:"

func (h *AuditLogHasher) hashValue(field, value string) (string, error) {
	if len(h.key) != 32 {
		return "", fmt.Errorf("audit HMAC key is unavailable")
	}
	var salt [16]byte
	if _, err := rand.Read(salt[:]); err != nil {
		return "", fmt.Errorf("failed to generate audit salt: %w", err)
	}
	mac := hmac.New(sha256.New, h.key)
	_, _ = mac.Write(salt[:])
	_, _ = mac.Write([]byte(field + "\x00" + value))
	return auditHashPrefix + hex.EncodeToString(salt[:]) + ":" + hex.EncodeToString(mac.Sum(nil)), nil
}

func isHMAC(value string) bool {
	body, ok := strings.CutPrefix(value, auditHashPrefix)
	if !ok {
		return false
	}
	salt, digest, ok := strings.Cut(body, ":")
	if !ok || len(salt) != 32 || len(digest) != 64 {
		return false
	}
	_, saltErr := hex.DecodeString(salt)
	_, digestErr := hex.DecodeString(digest)
	return saltErr == nil && digestErr == nil
}

// HashPIIForUser updates matching audit entries where the user is the actor or target.
func (h *AuditLogHasher) HashPIIForUser(ctx context.Context, tx pgx.Tx, userID int64) error {
	repo := repository.NewAuditLogRepository(tx)
	entries, err := repo.List(ctx, repository.AuditLogFilter{RelatedUserID: &userID})
	if err != nil {
		return fmt.Errorf("failed to list audit log entries for user: %w", err)
	}

	for _, entry := range entries {
		newPayload, changed, err := h.hashPIIInPayload(entry.Payload)
		if err != nil {
			return fmt.Errorf("failed to hash audit entry %d: %w", entry.ID, err)
		}

		if !changed {
			continue
		}

		if err := repo.UpdatePayload(ctx, entry.ID, newPayload); err != nil {
			return fmt.Errorf("failed to update audit entry %d: %w", entry.ID, err)
		}
	}

	return nil
}
