package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

var ErrInvalidKey = errors.New("encryption key must be 32 bytes (AES-256)")

type Encryptor struct {
	gcm     cipher.AEAD
	version int
	readers map[int]*Encryptor
}

const versionPrefix = "aesgcm:"

// NewVersionedEncryptor uses the active key for writes and version 1 for
// ciphertexts written before the version marker was introduced.
func NewVersionedEncryptor(active int, keys map[int][]byte) (*Encryptor, error) {
	readers := make(map[int]*Encryptor, len(keys))
	for version, key := range keys {
		if version < 1 {
			return nil, errors.New("key versions must be positive")
		}
		reader, err := NewEncryptor(key)
		if err != nil {
			return nil, err
		}
		readers[version] = reader
	}
	writer := readers[active]
	if writer == nil {
		return nil, errors.New("active key version is missing")
	}
	return &Encryptor{gcm: writer.gcm, version: active, readers: readers}, nil
}

func DeriveKey(key []byte, purpose string) []byte {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(purpose))
	return mac.Sum(nil)
}

func (e *Encryptor) header() string {
	if e.version == 0 {
		return ""
	}
	return versionPrefix + strconv.Itoa(e.version) + ":"
}

func NewEncryptor(key []byte) (*Encryptor, error) {
	if len(key) != 32 {
		return nil, ErrInvalidKey
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create gcm: %w", err)
	}

	return &Encryptor{gcm: gcm}, nil
}

func (e *Encryptor) Encrypt(plaintext string) (string, error) {
	nonce := make([]byte, e.gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("failed to generate nonce: %w", err)
	}

	header := e.header()
	ciphertext := e.gcm.Seal(nonce, nonce, []byte(plaintext), []byte(header))
	return header + base64.StdEncoding.EncodeToString(ciphertext), nil
}

func (e *Encryptor) Decrypt(encoded string) (string, error) {
	if e.version == 0 {
		return e.decrypt(encoded, "")
	}
	if !strings.HasPrefix(encoded, versionPrefix) {
		if legacy := e.readers[1]; legacy != nil {
			return legacy.decrypt(encoded, "")
		}
		return "", errors.New("legacy encryption key version 1 is missing")
	}
	versionText, body, ok := strings.Cut(strings.TrimPrefix(encoded, versionPrefix), ":")
	version, err := strconv.Atoi(versionText)
	if !ok || err != nil || version <= 0 || strconv.Itoa(version) != versionText {
		return "", errors.New("invalid ciphertext version")
	}
	reader := e.readers[version]
	if reader == nil {
		return "", errors.New("ciphertext key version is unavailable")
	}
	return reader.decrypt(body, versionPrefix+versionText+":")
}

// Reencrypt verifies even already-current ciphertext and preserves it unchanged.
func (e *Encryptor) Reencrypt(encoded string) (string, error) {
	plaintext, err := e.Decrypt(encoded)
	if err != nil {
		return "", err
	}
	if strings.HasPrefix(encoded, e.header()) {
		return encoded, nil
	}
	return e.Encrypt(plaintext)
}

func (e *Encryptor) decrypt(encoded, header string) (string, error) {
	ciphertext, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("failed to decode base64: %w", err)
	}

	nonceSize := e.gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return "", errors.New("ciphertext too short")
	}

	nonce, encrypted := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := e.gcm.Open(nil, nonce, encrypted, []byte(header))
	if err != nil {
		return "", fmt.Errorf("failed to decrypt: %w", err)
	}

	return string(plaintext), nil
}
