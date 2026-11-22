package config

import (
	"encoding/base64"
	"errors"
	"os"
	"strconv"
	"strings"

	"github.com/fastcheck/anonymus_bot/backend/internal/crypto"
)

type Encryption struct {
	PII      *crypto.Encryptor
	TOTP     *crypto.Encryptor
	AuditKey []byte
}

func LoadEncryption() (Encryption, error) {
	var cfg Encryption
	keys := make(map[int][]byte)
	if raw := os.Getenv("PII_ENCRYPTION_KEY"); raw != "" {
		key, err := base64.StdEncoding.Strict().DecodeString(raw)
		if err != nil || len(key) != 32 {
			return cfg, errors.New("PII_ENCRYPTION_KEY must be a base64-encoded 32-byte key")
		}
		keys[1] = key
	}
	if raw := os.Getenv("PII_ENCRYPTION_KEYS"); raw != "" {
		for entry := range strings.SplitSeq(raw, ",") {
			versionText, encoded, ok := strings.Cut(entry, ":")
			version, versionErr := strconv.Atoi(versionText)
			key, keyErr := base64.StdEncoding.Strict().DecodeString(encoded)
			if !ok || versionErr != nil || version < 2 || strconv.Itoa(version) != versionText ||
				keyErr != nil || len(key) != 32 || keys[version] != nil {
				return cfg, errors.New("PII_ENCRYPTION_KEYS must contain unique version:base64 pairs with versions >= 2 and 32-byte keys")
			}
			keys[version] = key
		}
	}
	active := 1
	if raw := os.Getenv("PII_ENCRYPTION_ACTIVE_VERSION"); raw != "" {
		version, err := strconv.Atoi(raw)
		if err != nil || version < 1 || strconv.Itoa(version) != raw {
			return cfg, errors.New("PII_ENCRYPTION_ACTIVE_VERSION must be a positive integer")
		}
		active = version
	}
	if keys[active] == nil {
		if active == 1 {
			return cfg, errors.New("PII_ENCRYPTION_KEY is required when PII_ENCRYPTION_ACTIVE_VERSION is 1")
		}
		return cfg, errors.New("PII_ENCRYPTION_ACTIVE_VERSION must be present in PII_ENCRYPTION_KEYS")
	}
	totp := make(map[int][]byte, len(keys))
	seen := make(map[[32]byte]bool, len(keys))
	for version, key := range keys {
		fingerprint := [32]byte(key)
		if seen[fingerprint] {
			return cfg, errors.New("PII_ENCRYPTION_KEY and PII_ENCRYPTION_KEYS must use different key material for each version")
		}
		seen[fingerprint] = true
		// Preserve the derivation used by existing totp:v1 values.
		totp[version] = crypto.DeriveKey(key, "operator TOTP encryption v1")
	}
	var err error
	cfg.PII, err = crypto.NewVersionedEncryptor(active, keys)
	if err != nil {
		return Encryption{}, err
	}
	cfg.TOTP, err = crypto.NewVersionedEncryptor(active, totp)
	if err != nil {
		return Encryption{}, err
	}
	cfg.AuditKey = crypto.DeriveKey(keys[active], "audit retention HMAC v1")
	return cfg, nil
}
