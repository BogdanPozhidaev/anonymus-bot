package handlers

import (
	"strings"
	"testing"
)

func TestValidInvitePayload(t *testing.T) {
	token := strings.Repeat("A", 43)
	if !validInvitePayload(token) {
		t.Fatal("valid invitation rejected")
	}
	for _, invalid := range []string{"", "42-c", "42-e", token + "=", token[:42] + "!", token[:42] + "B"} {
		if validInvitePayload(invalid) {
			t.Fatal("invalid invitation accepted")
		}
	}
}
