package crypto

import (
	"bytes"
	"strings"
	"testing"
)

func TestVersionedEncryption(t *testing.T) {
	key1 := bytes.Repeat([]byte{1}, 32)
	key2 := bytes.Repeat([]byte{2}, 32)
	legacy, err := NewEncryptor(key1)
	if err != nil {
		t.Fatal(err)
	}
	v1, err := NewVersionedEncryptor(1, map[int][]byte{1: key1})
	if err != nil {
		t.Fatal(err)
	}
	v2, err := NewVersionedEncryptor(2, map[int][]byte{1: key1, 2: key2})
	if err != nil {
		t.Fatal(err)
	}
	v2Only, err := NewVersionedEncryptor(2, map[int][]byte{2: key2})
	if err != nil {
		t.Fatal(err)
	}
	const plaintext = "fixture"
	for _, writer := range []*Encryptor{legacy, v1, v2} {
		stored, err := writer.Encrypt(plaintext)
		if err != nil {
			t.Fatal(err)
		}
		rotated, err := v2.Reencrypt(stored)
		if err != nil || !strings.HasPrefix(rotated, "aesgcm:2:") || (writer == v2) != (rotated == stored) {
			t.Fatalf("unexpected rotation result: %v", err)
		}
		if decoded, err := v2Only.Decrypt(rotated); err != nil || decoded != plaintext {
			t.Fatalf("new key could not read rotated value: %v", err)
		}
		if _, err := v1.Decrypt(rotated); err == nil {
			t.Fatal("old keyring accepted the new version")
		}
	}
	// Even if a key is accidentally shared, changing its version invalidates the tag.
	sameKey, err := NewVersionedEncryptor(2, map[int][]byte{1: key2, 2: key2})
	if err != nil {
		t.Fatal(err)
	}
	stored, err := sameKey.Encrypt(plaintext)
	if err != nil {
		t.Fatal(err)
	}
	for _, tampered := range []string{strings.Replace(stored, ":2:", ":1:", 1), strings.Replace(stored, ":2:", ":02:", 1), stored + "!"} {
		if _, err := sameKey.Reencrypt(tampered); err == nil {
			t.Fatal("tampered ciphertext accepted")
		}
	}
}
