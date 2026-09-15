package controller

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"github.com/gin-gonic/gin"
	"io"
	"net/http"
	"new-api-pilot/common"
	"new-api-pilot/constant"
	"new-api-pilot/dto"
	"strconv"
)

type balanceMonitorApplication interface {
	Ingest(context.Context, dto.BalanceIngest) error
	List(context.Context, dto.BalanceQuery) (dto.BalancePage, error)
}
type BalanceMonitorController struct {
	service balanceMonitorApplication
	token   string
}

func NewBalanceMonitorController(s balanceMonitorApplication, token string) *BalanceMonitorController {
	return &BalanceMonitorController{service: s, token: token}
}
func (c *BalanceMonitorController) Ingest(g *gin.Context) {
	expected := sha256.Sum256([]byte("Bearer " + c.token))
	actual := sha256.Sum256([]byte(g.GetHeader("Authorization")))
	if len(c.token) < 32 || subtle.ConstantTimeCompare(expected[:], actual[:]) != 1 {
		common.AbortError(g, http.StatusUnauthorized, constant.CodeAuthInvalid, "Authentication required", nil)
		return
	}
	g.Request.Body = http.MaxBytesReader(g.Writer, g.Request.Body, 2<<20)
	decoder := json.NewDecoder(g.Request.Body)
	decoder.DisallowUnknownFields()
	var v dto.BalanceIngest
	if e := decoder.Decode(&v); e != nil {
		common.AbortError(g, 400, constant.CodeValidationError, "Invalid batch", nil)
		return
	}
	if e := decoder.Decode(new(any)); e != io.EOF {
		common.AbortError(g, 400, constant.CodeValidationError, "Invalid batch", nil)
		return
	}
	if e := v.Validate(); e != nil {
		common.AbortError(g, 400, constant.CodeValidationError, "Invalid batch", nil)
		return
	}
	if e := c.service.Ingest(g.Request.Context(), v); e != nil {
		common.AbortInternalError(g)
		return
	}
	common.WriteSuccess(g, 200, gin.H{"accepted": len(v.Records)})
}
func (c *BalanceMonitorController) List(g *gin.Context) {
	page, _ := strconv.Atoi(g.DefaultQuery("p", "1"))
	size, _ := strconv.Atoi(g.DefaultQuery("page_size", "50"))
	q := dto.BalanceQuery{Kind: g.DefaultQuery("kind", "current"), Start: g.Query("start"), End: g.Query("end"), AccountID: g.Query("account_id"), SiteID: g.Query("site_id"), Page: page, PageSize: size}
	if e := q.Validate(); e != nil {
		common.AbortError(g, 400, constant.CodeValidationError, "Invalid query", nil)
		return
	}
	result, e := c.service.List(g.Request.Context(), q)
	if e != nil {
		common.AbortInternalError(g)
		return
	}
	common.WriteSuccess(g, 200, result)
}
func (c *BalanceMonitorController) Accounts(g *gin.Context) {
	result, e := c.service.List(g.Request.Context(), dto.BalanceQuery{Kind: "current", Page: 1, PageSize: 200})
	if e != nil {
		common.AbortInternalError(g)
		return
	}
	common.WriteSuccess(g, 200, result)
}
