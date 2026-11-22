package auth

import (
	"errors"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"
)

const bcryptCost = 12

func ValidateNewPassword(password string) error {
	if len(password) > 72 || !utf8.ValidString(password) || utf8.RuneCountInString(password) < 12 ||
		strings.TrimSpace(password) == "" || strings.ContainsFunc(password, unicode.IsControl) {
		return errors.New("password must contain at least 12 characters, fit in 72 UTF-8 bytes and contain no control characters")
	}
	return nil
}

func HashPassword(plaintext string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(plaintext), bcryptCost)
	if err != nil {
		return "", fmt.Errorf("failed to hash password: %w", err)
	}
	return string(hash), nil
}

func VerifyPassword(hash, plaintext string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(plaintext))
	return err == nil
}
