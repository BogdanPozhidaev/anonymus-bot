package api

import (
	"bytes"
	"crypto/subtle"
	"errors"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/fastcheck/anonymus_bot/backend/internal/models"
)

// The API receives JSON and media references. File contents travel separately.
const maxJSONBodySize = 64 << 10

func limitRequestBody(c *gin.Context) {
	req := c.Request
	if req.Body == nil || req.Body == http.NoBody {
		return
	}
	if req.ContentLength > maxJSONBodySize {
		c.AbortWithStatusJSON(http.StatusRequestEntityTooLarge, gin.H{"error": "request body too large"})
		return
	}
	reader := http.MaxBytesReader(c.Writer, req.Body, maxJSONBodySize)
	body, err := io.ReadAll(reader)
	_ = reader.Close()
	if err != nil {
		var limit *http.MaxBytesError
		if errors.As(err, &limit) {
			c.AbortWithStatusJSON(http.StatusRequestEntityTooLarge, gin.H{"error": "request body too large"})
		} else {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		}
		return
	}
	req.Body = io.NopCloser(bytes.NewReader(body))
}

func requireInternalToken(token string) gin.HandlerFunc {
	expected := []byte(token)
	return func(c *gin.Context) {
		values := c.Request.Header.Values("X-Internal-Token")
		// Gin's recovery middleware can dump request headers after a handler panic.
		c.Request.Header.Del("X-Internal-Token")
		if len(expected) != 64 || len(values) != 1 || subtle.ConstantTimeCompare([]byte(values[0]), expected) != 1 {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
		c.Next()
	}
}

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

		role, status, version, err := s.operatorRepo.GetAuthState(c.Request.Context(), data.OperatorID)
		if err != nil || status != models.OperatorStatusActive || version != data.AuthVersion || role != data.Role {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid or expired session"})
			return
		}

		c.Set(contextKeyOperatorID, data.OperatorID)
		c.Set(contextKeyRole, role)
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
