package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"strings"
)

const recoveryCodeCount = 8

func GenerateRecoveryCodes() ([]string, [][]byte) {
	codes := make([]string, recoveryCodeCount)
	digests := make([][]byte, recoveryCodeCount)
	for i := range codes {
		codes[i] = rand.Text()
		digests[i] = RecoveryCodeDigest(codes[i])
	}
	return codes, digests
}

func RecoveryCodeDigest(code string) []byte {
	digest := sha256.Sum256([]byte(strings.ToUpper(strings.TrimSpace(code))))
	return digest[:]
}
