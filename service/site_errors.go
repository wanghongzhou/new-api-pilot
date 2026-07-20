package service

import (
	"errors"
	"strconv"

	"new-api-pilot/model"
)

type SiteConfigChangedError struct {
	SiteID                int64
	ExpectedConfigVersion int
	ActualConfigVersion   int
}

func (err *SiteConfigChangedError) Error() string {
	if err == nil {
		return ErrSiteConfigChanged.Error()
	}
	return ErrSiteConfigChanged.Error() + " for site " + strconv.FormatInt(err.SiteID, 10)
}

func (err *SiteConfigChangedError) Unwrap() error { return ErrSiteConfigChanged }

func newSiteConfigChangedError(siteID int64, expected, actual int) error {
	return &SiteConfigChangedError{
		SiteID: siteID, ExpectedConfigVersion: expected, ActualConfigVersion: actual,
	}
}

var (
	ErrSiteNotFound             = errors.New("site not found")
	ErrSiteConflict             = errors.New("site base URL already exists")
	ErrSiteInvalidBaseURL       = errors.New("site base URL is invalid")
	ErrSiteDeleteRestricted     = errors.New("site has associated history or accounts")
	ErrSiteInvalidState         = errors.New("site state does not allow this operation")
	ErrSiteInvalidBackfillRange = errors.New("site backfill range is invalid")
	ErrSiteInvalidStatisticsEnd = errors.New("site statistics end is invalid")
	ErrSiteResourceRange        = errors.New("site resource range is invalid")
	ErrSiteConfigChanged        = errors.New("site configuration changed")
	ErrBaseURLPreflightRequired = errors.New("base URL preflight is required")
	ErrSiteExportDisabled       = errors.New("site data export is disabled")
	ErrSiteIncompatible         = errors.New("site identity or contract is incompatible")
	ErrSiteCapabilitiesPending  = errors.New("site required capabilities are not ready")
	ErrSiteTaskOverlap          = errors.New("site has an overlapping active task")
)

type SiteDeleteRestrictedError struct {
	DependencyTypes []model.SiteDeleteDependencyType
}

func (err *SiteDeleteRestrictedError) Error() string { return ErrSiteDeleteRestricted.Error() }

func (err *SiteDeleteRestrictedError) Unwrap() error { return ErrSiteDeleteRestricted }
