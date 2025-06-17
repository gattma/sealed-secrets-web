package handler

import (
	"github.com/gin-gonic/gin"
	"net/http"
)

func (h *Handler) ShowLoginPage(c *gin.Context) {
	// TODO check if already logged in => redirect to dashboard
	c.HTML(http.StatusOK, "login.html", nil)
}
