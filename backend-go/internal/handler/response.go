package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// fail 统一错误响应：{"detail": "..."}
func fail(c *gin.Context, code int, msg string) {
	c.AbortWithStatusJSON(code, gin.H{"detail": msg})
}

// badRequest 400
func badRequest(c *gin.Context, msg string) {
	fail(c, http.StatusBadRequest, msg)
}

// notFound 404
func notFound(c *gin.Context, msg string) {
	fail(c, http.StatusNotFound, msg)
}

// forbidden 403
func forbidden(c *gin.Context, msg string) {
	fail(c, http.StatusForbidden, msg)
}
