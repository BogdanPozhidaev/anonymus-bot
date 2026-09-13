package auth

import (
	"fmt"

	"github.com/pquerna/otp/totp"
)

// GenerateTOTPSecret создаёт новый секрет для оператора и URL для QR-кода,
// который оператор отсканирует в приложении-аутентификаторе (Google Authenticator и т.п.).
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
