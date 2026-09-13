package api

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/fastcheck/anonymus_bot/backend/internal/models"
)

const (
	contextKeyOperatorID = "operator_id"
	contextKeyRole       = "operator_role"
)

// requireAuth проверяет наличие валидной сессии оператора в заголовке
// Authorization: Bearer <token>. При успехе кладёт operator_id и role
// в контекст запроса для последующего использования хендлерами.
func (s *Server) requireAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		token := extractSessionToken(c)
		if token == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing session token"})
			return
		}

		data, err := s.sessionService.Get(c.Request.Context(), token)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid or expired session"})
			return
		}

		c.Set(contextKeyOperatorID, data.OperatorID)
		c.Set(contextKeyRole, data.Role)
		c.Next()
	}
}

// requireAdmin — дополнительная проверка после requireAuth, ограничивает
// доступ только для роли admin.
func (s *Server) requireAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		role, exists := c.Get(contextKeyRole)
		if !exists || role != models.OperatorRoleAdmin {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "admin access required"})
			return
		}
		c.Next()
	}
}

func getOperatorID(c *gin.Context) (int64, bool) {
	val, exists := c.Get(contextKeyOperatorID)
	if !exists {
		return 0, false
	}
	id, ok := val.(int64)
	return id, ok
}

func getOperatorRole(c *gin.Context) (string, bool) {
	val, exists := c.Get(contextKeyRole)
	if !exists {
		return "", false
	}
	role, ok := val.(string)
	return role, ok
}
