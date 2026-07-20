package controller

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"

	"new-api-pilot/common"
	"new-api-pilot/dto"
)

type DingTalkTester interface {
	Test(context.Context, string) (dto.NotificationTestResult, error)
}

type NotificationController struct{ dingTalk DingTalkTester }

func NewNotificationController(dingTalk DingTalkTester) *NotificationController {
	if dingTalk == nil {
		return nil
	}
	return &NotificationController{dingTalk: dingTalk}
}

func (controller *NotificationController) TestDingTalk(c *gin.Context) {
	if !requireNoQuery(c) || !requireEmptyBody(c) {
		return
	}
	result, err := controller.dingTalk.Test(c.Request.Context(), common.RequestID(c))
	if err != nil {
		common.AbortInternalError(c)
		return
	}
	common.WriteSuccess(c, http.StatusOK, result)
}
