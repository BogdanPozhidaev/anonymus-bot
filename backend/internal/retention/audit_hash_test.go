package retention

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"
)

func TestAuditPIIUsesSaltedHMAC(t *testing.T) {
	key := bytes.Repeat([]byte{1}, 32)
	hasher := NewAuditLogHasher(key)
	payload := []byte(`{"phone":"fixture","email":"fixture","first_name":null,"telegram_id":9007199254740993,"other":"kept"}`)
	first, changed, err := hasher.hashPIIInPayload(payload)
	if err != nil || !changed || bytes.Contains(first, []byte("fixture")) {
		t.Fatalf("PII was not replaced: %v", err)
	}
	var data map[string]any
	if err := json.Unmarshal(first, &data); err != nil {
		t.Fatal(err)
	}
	if data["first_name"] != nil || data["other"] != "kept" || data["phone"] == data["email"] {
		t.Fatal("unexpected payload after hashing")
	}
	value, ok := data["telegram_id"].(string)
	if !ok || !isHMAC(value) {
		t.Fatal("unexpected digest format")
	}
	salt, digest, _ := strings.Cut(strings.TrimPrefix(value, auditHashPrefix), ":")
	saltBytes, _ := hex.DecodeString(salt)
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write(saltBytes)
	_, _ = mac.Write([]byte("telegram_id\x009007199254740993"))
	if digest != hex.EncodeToString(mac.Sum(nil)) {
		t.Fatal("digest did not authenticate the original field and number")
	}
	again, changed, err := hasher.hashPIIInPayload(first)
	if err != nil || changed || !bytes.Equal(again, first) {
		t.Fatal("reprocessing changed an existing HMAC")
	}
	fresh, changed, err := hasher.hashPIIInPayload(payload)
	if err != nil || !changed || bytes.Equal(fresh, first) {
		t.Fatal("repeat input did not receive a fresh salt")
	}
}
