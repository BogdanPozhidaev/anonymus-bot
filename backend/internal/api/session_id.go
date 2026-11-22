package api

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

func readSessionID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid session id"})
		return 0, false
	}
	return id, true
}
