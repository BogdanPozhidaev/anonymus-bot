package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type pageQuery struct {
	Limit  int `form:"limit,default=50" binding:"min=1,max=200"`
	Offset int `form:"offset" binding:"min=0,max=1000000"`
}

func readPage(c *gin.Context) (pageQuery, bool) {
	var page pageQuery
	if c.ShouldBindQuery(&page) != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid pagination"})
		return page, false
	}
	return page, true
}

func trimPage[T any](items []T, limit int) ([]T, bool) {
	if len(items) > limit {
		return items[:limit], true
	}
	return items, false
}
