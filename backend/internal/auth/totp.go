package auth

import (
	"fmt"

	"github.com/pquerna/otp/totp"
)

// GenerateTOTPSecret returns a new secret and an enrollment URL for an authenticator.
func GenerateTOTPSecret(operatorLogin, issuer string) (secret string, otpAuthURL string, err error) {
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      issuer,
		AccountName: operatorLogin,
	})
	if err != nil {
		return "", "", fmt.Errorf("failed to generate totp secret: %w", err)
	}

	return key.Secret(), key.URL(), nil
}

func VerifyTOTPCode(secret, code string) bool {
	return totp.Validate(code, secret)
}
