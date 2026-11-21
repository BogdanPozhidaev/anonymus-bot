package api

import (
	"testing"

	"github.com/fastcheck/anonymus_bot/backend/internal/botclient"
)

func TestNewServerPreservesBotClientAndMediaDirs(t *testing.T) {
	client := botclient.New("http://bot.invalid")
	const photosDir = "test-media/photos"
	const voiceDir = "test-media/voice"

	server := NewServer(
		nil, nil, nil, // Database, Redis, and logger.
		nil, nil, nil, nil, nil, nil, // Repositories.
		nil, nil, nil, // Moderation and authentication services.
		client, photosDir, voiceDir,
	)

	if server.botClient != client {
		t.Errorf("botClient = %p, want %p", server.botClient, client)
	}
	if server.photosDir != photosDir {
		t.Errorf("photosDir = %q, want %q", server.photosDir, photosDir)
	}
	if server.voiceDir != voiceDir {
		t.Errorf("voiceDir = %q, want %q", server.voiceDir, voiceDir)
	}
}
