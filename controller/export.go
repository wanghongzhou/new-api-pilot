package controller

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"new-api-pilot/common"
	"new-api-pilot/constant"
	"new-api-pilot/dto"
	"new-api-pilot/middleware"
	"new-api-pilot/service"
)

const exportCreateBodyLimit = 64 * 1024

type exportApplication interface {
	Create(context.Context, string, dto.ExportCreateRequest) (dto.ExportJobItem, error)
	List(context.Context, string, dto.ExportListQuery) (common.PageData[dto.ExportJobItem], error)
	Get(context.Context, string, int64) (dto.ExportJobItem, error)
	OpenDownload(context.Context, string, int64) (service.ExportDownload, error)
}

type ExportController struct {
	exports exportApplication
}

func NewExportController(exports exportApplication) *ExportController {
	return &ExportController{exports: exports}
}

func (controller *ExportController) Create(c *gin.Context) {
	identity, ok := middleware.CurrentIdentity(c)
	if controller == nil || controller.exports == nil || !ok {
		common.AbortInternalError(c)
		return
	}
	if fields := validateQueryKeys(c, map[string]struct{}{}); len(fields) > 0 {
		common.AbortError(c, http.StatusBadRequest, constant.CodeValidationError, "Invalid export query", fields)
		return
	}
	var request dto.ExportCreateRequest
	if !decodeJSON(c, &request, exportCreateBodyLimit) {
		return
	}
	request.Normalize()
	if fields := request.Validate(); fields != nil {
		common.AbortError(c, http.StatusBadRequest, constant.CodeValidationError, "Invalid export request", fields)
		return
	}
	item, err := controller.exports.Create(c.Request.Context(), identity.ID, request)
	if err != nil {
		writeExportError(c, 0, err)
		return
	}
	common.WriteSuccess(c, http.StatusOK, item)
}

func (controller *ExportController) List(c *gin.Context) {
	identity, ok := middleware.CurrentIdentity(c)
	if controller == nil || controller.exports == nil || !ok {
		common.AbortInternalError(c)
		return
	}
	query, fields := parseExportListQuery(c)
	if fields != nil {
		common.AbortError(c, http.StatusBadRequest, constant.CodeValidationError, "Invalid export query", fields)
		return
	}
	page, err := controller.exports.List(c.Request.Context(), identity.ID, query)
	if err != nil {
		writeExportError(c, 0, err)
		return
	}
	common.WriteSuccess(c, http.StatusOK, page)
}

func (controller *ExportController) Detail(c *gin.Context) {
	controller.read(c, false)
}

func (controller *ExportController) Download(c *gin.Context) {
	controller.read(c, true)
}

func (controller *ExportController) read(c *gin.Context, download bool) {
	identity, ok := middleware.CurrentIdentity(c)
	if controller == nil || controller.exports == nil || !ok {
		common.AbortInternalError(c)
		return
	}
	if fields := validateQueryKeys(c, map[string]struct{}{}); len(fields) > 0 {
		common.AbortError(c, http.StatusBadRequest, constant.CodeValidationError, "Invalid export query", fields)
		return
	}
	id, valid := parsePositiveID(c.Param("id"))
	if !valid {
		common.AbortError(c, http.StatusBadRequest, constant.CodeValidationError, "Invalid export ID", map[string]string{"id": "must be a positive ID"})
		return
	}
	if !download {
		item, err := controller.exports.Get(c.Request.Context(), identity.ID, id)
		if err != nil {
			writeExportError(c, id, err)
			return
		}
		common.WriteSuccess(c, http.StatusOK, item)
		return
	}
	file, err := controller.exports.OpenDownload(c.Request.Context(), identity.ID, id)
	if err != nil {
		writeExportError(c, id, err)
		return
	}
	defer func() { _ = file.File.Close() }()
	c.Header("Content-Type", file.ContentType)
	c.Header("Content-Disposition", file.ContentDisposition)
	c.Header("Content-Length", strconv.FormatInt(file.Size, 10))
	c.Header("X-Content-Type-Options", "nosniff")
	c.Status(http.StatusOK)
	if err := service.CopyExportDownload(c.Writer, file); err != nil {
		_ = c.Error(err)
	}
}

func parseExportListQuery(c *gin.Context) (dto.ExportListQuery, map[string]string) {
	fields := validateQueryKeysAllowRepeated(c, map[string]struct{}{
		"p": {}, "page_size": {}, "status": {}, "format": {}, "statistics_type": {}, "sort_by": {}, "sort_order": {},
	}, map[string]struct{}{"status": {}})
	query := dto.ExportListQuery{Page: 1, PageSize: 20, SortBy: "created_at", SortOrder: "desc"}
	parsePageQuery(c, &query.Page, &query.PageSize, fields)
	query.Statuses = parseRepeatedEnumQuery(c, "status", fields, func(value string) bool {
		return value == dto.ExportStatusPending || value == dto.ExportStatusRunning ||
			value == dto.ExportStatusSuccess || value == dto.ExportStatusFailed || value == dto.ExportStatusExpired
	})
	query.Format, _ = singletonQueryValue(c, "format", fields)
	query.StatisticsType, _ = singletonQueryValue(c, "statistics_type", fields)
	query.SortBy, _ = singletonQueryValue(c, "sort_by", fields)
	query.SortOrder, _ = singletonQueryValue(c, "sort_order", fields)
	query.Normalize()
	mergeFieldErrors(fields, query.Validate())
	if len(fields) > 0 {
		return dto.ExportListQuery{}, fields
	}
	return query, nil
}

func writeExportError(c *gin.Context, exportID int64, err error) {
	params := func() map[string]any {
		if exportID <= 0 {
			return nil
		}
		return map[string]any{"export_id": strconv.FormatInt(exportID, 10)}
	}
	switch {
	case errors.Is(err, service.ErrExportInvalid):
		common.AbortError(c, http.StatusBadRequest, constant.CodeValidationError, "Invalid export request", map[string]string{"export": "request is invalid"})
	case errors.Is(err, service.ErrExportNotFound):
		common.AbortError(c, http.StatusNotFound, constant.CodeNotFound, "Export job not found", nil)
	case errors.Is(err, service.ErrExportLimitReached):
		common.AbortError(c, http.StatusConflict, constant.CodeExportLimitReached, "Export task limit reached", nil)
	case errors.Is(err, service.ErrExportDiskLow):
		var disk *service.ExportDiskLowError
		errorParams := map[string]any{}
		if errors.As(err, &disk) {
			errorParams["free_bytes"] = strconv.FormatUint(disk.FreeBytes, 10)
			errorParams["threshold_bytes"] = strconv.FormatInt(disk.ThresholdBytes, 10)
		}
		if exportID > 0 {
			errorParams["export_id"] = strconv.FormatInt(exportID, 10)
		}
		common.AbortErrorWithParams(c, http.StatusInsufficientStorage, string(constant.MessageExportDiskLow), "Export disk space is low", errorParams, nil)
	case errors.Is(err, service.ErrExportNotReady):
		common.AbortError(c, http.StatusConflict, constant.CodeConflict, "Export file is not ready", nil)
	case errors.Is(err, service.ErrExportExpired):
		common.AbortErrorWithParams(c, http.StatusGone, constant.CodeExportExpired, "Export file has expired", params(), nil)
	case errors.Is(err, service.ErrExportFileMissing):
		common.AbortErrorWithParams(c, http.StatusGone, constant.CodeExportFileMissing, "Export file is missing", params(), nil)
	default:
		common.AbortInternalError(c)
	}
}
