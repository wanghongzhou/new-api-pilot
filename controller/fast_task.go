package controller

import (
	"net/http"
	"strconv"
	"github.com/gin-gonic/gin"
	"new-api-pilot/common"
	"new-api-pilot/constant"
	"new-api-pilot/dto"
)

type FastTaskController struct { Store *common.RedisStore }
func NewFastTaskController(store *common.RedisStore) *FastTaskController { return &FastTaskController{Store: store} }
func (ctl *FastTaskController) List(c *gin.Context) {
	if ctl == nil || ctl.Store == nil { common.AbortError(c,http.StatusServiceUnavailable,constant.CodeInternalError,"Fast task history unavailable",nil); return }
	siteID,_ := strconv.ParseInt(c.Query("site_id"),10,64); if siteID<=0 { common.AbortError(c,400,constant.CodeValidationError,"site_id is required",nil); return }
	taskType:=c.Query("task_type"); if taskType=="" { common.AbortError(c,400,constant.CodeValidationError,"task_type is required",nil); return }
	switch taskType { case constant.TaskTypeSiteProbe, constant.TaskTypeRealtimeStat, constant.TaskTypeResourceSnapshot: default: common.AbortError(c,400,constant.CodeValidationError,"unsupported task_type",nil); return }
	statusFilter:=c.Query("status")
	offset,_:=strconv.Atoi(c.DefaultQuery("offset","0")); limit,_:=strconv.Atoi(c.DefaultQuery("limit","50")); if offset<0{offset=0}; if limit<=0||limit>100{limit=50}
	rows,total,hasMore,err:=ctl.Store.ListFiltered(c.Request.Context(),siteID,taskType,statusFilter,offset,limit); if err!=nil { common.AbortError(c,500,constant.CodeInternalError,"Failed to load fast task history",nil); return }
	out:=make([]dto.FastTaskHistoryItem,0,len(rows)); for _,r:=range rows { out=append(out,dto.FastTaskHistoryItem{SiteID:strconv.FormatInt(r.SiteID,10),TaskType:r.TaskType,StartedAt:r.StartedAt,FinishedAt:r.FinishedAt,Status:r.Status,DurationMS:r.DurationMS,Error:r.Error,RequestID:r.RequestID}) }
	common.WriteSuccess(c,http.StatusOK,gin.H{"items":out,"offset":offset,"limit":limit,"total":total,"has_more":hasMore})
}
