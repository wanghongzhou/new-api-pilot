package router

import (
	"github.com/gin-gonic/gin"
	"new-api-pilot/constant"
	"new-api-pilot/controller"
	"new-api-pilot/middleware"
	"testing"
)

type modelCatalogResolver struct{}

func (modelCatalogResolver) ResolveIdentity(*gin.Context) (middleware.Identity, error) {
	return middleware.Identity{ID: "1", Role: constant.RoleViewer, Status: constant.UserStatusEnabled}, nil
}
func TestModelCatalogRoutesReadOnly(t *testing.T) {
	gin.SetMode(gin.TestMode)
	e := gin.New()
	RegisterModelCatalogRoutes(e, controller.NewModelCatalogController(nil), modelCatalogResolver{})
	if len(e.Routes()) != 6 {
		t.Fatalf("routes=%v", e.Routes())
	}
	for _, r := range e.Routes() {
		if r.Method != "GET" {
			t.Fatalf("mutation route=%s %s", r.Method, r.Path)
		}
	}
}
