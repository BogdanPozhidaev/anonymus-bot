package api

import (
	"context"
	"sync"
	"time"

	"github.com/fastcheck/anonymus_bot/backend/internal/botclient"
	"github.com/fastcheck/anonymus_bot/backend/internal/models"
)

func (s *Server) notifySessionStatus(ctx context.Context, session *models.Session, status string) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var sends sync.WaitGroup
	sent := make(map[int64]bool, 2)
	for _, userID := range []*int64{session.ClientUserID, session.ExecutorUserID} {
		if userID == nil || sent[*userID] {
			continue
		}
		sent[*userID] = true
		sends.Go(func() {
			user, err := s.userRepo.GetByID(ctx, *userID)
			if err != nil || user.TelegramID <= 0 {
				s.logger.Warn("cannot resolve recipient for session notice", "session_id", session.ID)
				return
			}
			text := "Сессия приостановлена."
			if status == models.SessionStatusClosed {
				text = "Сессия закрыта."
			}
			if user.LanguageCode == "en" {
				text = "The session has been paused."
				if status == models.SessionStatusClosed {
					text = "The session has been closed."
				}
			}
			if err := s.botClient.SendMessage(ctx, botclient.SendMessageRequest{RecipientTelegramID: user.TelegramID, Text: text}); err != nil {
				s.logger.Warn("session notice delivery failed", "session_id", session.ID)
			}
		})
	}
	sends.Wait()
}
