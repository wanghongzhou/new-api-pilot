package service

import (
	"context"
	"errors"

	"new-api-pilot/common"
	"new-api-pilot/constant"
	"new-api-pilot/model"
)

func expireSiteAuthorization(
	ctx context.Context,
	sites *model.SiteRepository,
	clock common.Clock,
	postCommit PostCommitNotifier,
	siteID int64,
	expectedConfigVersion int,
) error {
	if sites == nil || clock == nil || siteID <= 0 || expectedConfigVersion <= 0 {
		return model.ErrCollectionRunContract
	}
	committedAt := int64(0)
	err := sites.WithTransaction(ctx, func(repository *model.SiteRepository) error {
		site, err := repository.FindByIDForUpdate(ctx, siteID)
		if err != nil {
			return err
		}
		if site.ConfigVersion != expectedConfigVersion {
			if site.AuthStatus == constant.SiteAuthExpired && site.ConfigVersion == expectedConfigVersion+1 {
				return nil
			}
			return model.ErrSiteRunConfigChanged
		}
		if site.AuthStatus == constant.SiteAuthExpired {
			return nil
		}
		if site.AuthStatus != constant.SiteAuthAuthorized {
			return model.ErrSiteRunConfigChanged
		}
		now := clock.Now().Unix()
		if now <= 0 {
			return model.ErrCollectionRunContract
		}
		committedAt = monotonicMutationTime(now, site.UpdatedAt)
		if err := repository.BumpSiteFence(ctx, &site, committedAt); err != nil {
			return err
		}
		site.AuthStatus = constant.SiteAuthExpired
		if site.ManagementStatus == constant.SiteManagementDisabled {
			site.StatisticsStatus = constant.SiteStatisticsPaused
		} else {
			site.StatisticsStatus = constant.SiteStatisticsError
		}
		site.UpdatedAt = committedAt
		return repository.Save(ctx, &site)
	})
	if err != nil {
		return err
	}
	if postCommit != nil && committedAt > 0 {
		postCommit.NotifyAfterCommit(ctx, AlertPostCommitTrigger{
			Source: AlertSampleSourceAuth, SiteID: siteID, ObservedAt: committedAt,
		})
	}
	return nil
}

func upstreamAuthorizationFailure(err error) bool {
	return errors.Is(err, ErrUpstreamAuthExpired) || errors.Is(err, ErrUpstreamPermissionDenied)
}
