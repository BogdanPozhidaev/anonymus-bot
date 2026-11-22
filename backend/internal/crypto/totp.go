package crypto

import (
	"errors"
	"strings"
)

const totpSecretPrefix = "totp:v1:"

func (e *Encryptor) EncryptTOTP(secret string) (string, error) {
	encoded, err := e.Encrypt(secret)
	if err != nil {
		return "", err
	}
	return totpSecretPrefix + encoded, nil
}

func (e *Encryptor) DecryptTOTP(stored string) (string, bool, error) {
	body, encrypted := strings.CutPrefix(stored, totpSecretPrefix)
	if !encrypted {
		if strings.Contains(stored, ":") {
			return "", false, errors.New("invalid TOTP ciphertext format")
		}
		return stored, true, nil
	}
	secret, err := e.Decrypt(body)
	return secret, false, err
}

func (e *Encryptor) ReencryptTOTP(stored string) (string, error) {
	if body, encrypted := strings.CutPrefix(stored, totpSecretPrefix); encrypted {
		encoded, err := e.Reencrypt(body)
		if err != nil {
			return "", err
		}
		return totpSecretPrefix + encoded, nil
	}
	secret, _, err := e.DecryptTOTP(stored)
	if err != nil || secret == "" {
		return stored, err
	}
	return e.EncryptTOTP(secret)
}
