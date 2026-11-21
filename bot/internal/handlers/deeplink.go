package handlers

import "encoding/base64"

func validInvitePayload(payload string) bool {
	if len(payload) != base64.RawURLEncoding.EncodedLen(32) {
		return false
	}
	_, err := base64.RawURLEncoding.Strict().DecodeString(payload)
	return err == nil
}
