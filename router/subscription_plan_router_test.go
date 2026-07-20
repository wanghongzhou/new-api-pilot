package router

import (
	"github.com/gin-gonic/gin"
	"new-api-pilot/constant"
	"new-api-pilot/controller"
	"new-api-pilot/middleware"
	"testing"
)

type subscriptionPlanResolver struct{}

func (subscriptionPlanResolver) ResolveIdentity(*gin.Context) (middleware.Identity, error) {
	return middleware.Identity{ID: "1", Role: constant.RoleViewer, Status: constant.UserStatusEnabled}, nil
}
func TestSubscriptionPlanRoutesReadOnly(t *testing.T) {
	gin.SetMode(gin.TestMode)
	e := gin.New()
	RegisterSubscriptionPlanRoutes(e, controller.NewSubscriptionPlanController(nil), subscriptionPlanResolver{})
	if len(e.Routes()) != 4 {
		t.Fatalf("routes=%v", e.Routes())
	}
	for _, r := range e.Routes() {
		if r.Method != "GET" {
			t.Fatalf("mutation=%s", r.Method)
		}
	}
}
