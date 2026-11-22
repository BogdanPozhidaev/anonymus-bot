package repository_test

import (
	"io"
	"log/slog"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fastcheck/anonymus_bot/backend/internal/api"
	"github.com/fastcheck/anonymus_bot/backend/internal/moderation"
	"github.com/fastcheck/anonymus_bot/backend/internal/repository"
	"github.com/gin-gonic/gin"
)

func TestK6AuthenticatedRelay(t *testing.T) {
	if os.Getenv("RUN_K6_RELAY") != "1" {
		t.Skip("set RUN_K6_RELAY=1 to run the isolated k6 workload")
	}
	k6, err := exec.LookPath("k6")
	if err != nil {
		t.Fatal(err)
	}
	ctx := t.Context()
	pool := setupTestDB(t)
	redis := setupThrottleRedis(t)
	scripts := filepath.Join("..", "..", "scripts")
	seed, err := os.ReadFile(filepath.Join(scripts, "seed_load_test.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, string(seed)); err != nil {
		t.Fatal(err)
	}
	token := strings.Repeat("a", 64)
	tokenFile := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(tokenFile, []byte(token+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	api.NewServer(pool, redis, slog.New(slog.NewTextHandler(io.Discard, nil)),
		repository.NewUserRepository(pool, nil), repository.NewSessionRepository(pool), nil, nil,
		repository.NewMessageRepository(pool), nil, moderation.NewService(redis, moderation.Config{}),
		nil, nil, nil, "", "").RegisterRoutes(router, token)
	server := httptest.NewServer(router)
	t.Cleanup(server.Close)

	cmd := exec.CommandContext(ctx, k6, "run", "--no-usage-report", "--no-color", filepath.Join(scripts, "load_test.js"))
	cmd.Env = append(cmd.Environ(), "BASE_URL="+server.URL, "INTERNAL_API_TOKEN_FILE="+tokenFile)
	output, err := cmd.CombinedOutput()
	t.Logf("k6 results:\n%s", output)
	if err != nil {
		t.Fatal(err)
	}
	var messages, sessions, senderMismatches int
	err = pool.QueryRow(ctx, `SELECT count(*), count(DISTINCT m.session_id),
		count(*) FILTER (WHERE m.sender_user_id IS DISTINCT FROM s.client_user_id)
		FROM message_outbox o JOIN messages m ON m.id = o.message_id
		JOIN sessions s ON s.id = m.session_id WHERE o.status = 'queued' AND NOT m.delivered`).
		Scan(&messages, &sessions, &senderMismatches)
	if err != nil || messages != 250 || sessions != 50 || senderMismatches != 0 {
		t.Fatalf("unexpected queued workload: %d messages in %d sessions, %d sender mismatches: %v", messages, sessions, senderMismatches, err)
	}
}
