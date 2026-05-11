package apis

import (
	"navmesh-go/services"

	"github.com/gin-gonic/gin"
	"github.com/wfu-work/nav-common-go-lib/response"
)

type AuthApi struct{}

func (a AuthApi) Login(c *gin.Context) {
	var req services.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}
	result, err := authService.Login(req)
	if err != nil {
		auditService.Record(services.AuditInput{Actor: req.Username, Action: "login_failed", Resource: "auth", Message: err.Error(), SourceIP: c.ClientIP()})
		response.FailWithMessage(err.Error(), c)
		return
	}
	auditService.Record(services.AuditInput{Actor: result.User.Username, Action: "login", Resource: "auth", ResourceID: result.User.Username, SourceIP: c.ClientIP()})
	response.Ok(result, c)
}

func (a AuthApi) Profile(c *gin.Context) {
	user, err := authService.Profile(actorName(c))
	if err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}
	response.Ok(user, c)
}

func (a AuthApi) ChangePassword(c *gin.Context) {
	var req services.ChangePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}
	if err := authService.ChangePassword(actorName(c), req); err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}
	auditService.Record(services.AuditInput{Actor: actorName(c), Action: "change_password", Resource: "user", ResourceID: actorName(c), SourceIP: c.ClientIP()})
	response.Ok(true, c)
}
