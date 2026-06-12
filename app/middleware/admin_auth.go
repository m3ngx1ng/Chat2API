package middleware

import (
	"chat2api/app/common"
	"chat2api/app/conf"
	"strings"

	"github.com/gin-gonic/gin"
)

func AdminAuth(c *gin.Context) {
	token, err := c.Cookie(conf.AdminSessionCookieName())
	if err != nil || !conf.ValidateAdminSessionToken(strings.TrimSpace(token)) {
		common.ErrorResponse(c, 401, "admin login required", nil)
		return
	}
	c.Next()
}
