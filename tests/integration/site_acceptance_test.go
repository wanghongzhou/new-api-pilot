package integration_test

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	"new-api-pilot/constant"
	"new-api-pilot/dto"
	"new-api-pilot/model"
	"new-api-pilot/service"
	testsupport "new-api-pilot/tests/support"
)

func TestA04A06A07A10A24A36A37A56A57A86SiteAcceptance(t *testing.T) {
	database := openCoreAcceptanceTransaction(t)
	now := int64(1_768_622_400)
	clock := testsupport.NewFakeClock(time.Unix(now, 0))
	cipher := newCoreCipher(t)
	client := newCoreSiteClient(now)
	factory := &coreSiteClientFactory{client: client}
	sites, err := service.NewSiteService(service.SiteServiceOptions{
		Repository: model.NewSiteRepository(database), ClientFactory: factory, Cipher: cipher, Clock: clock,
		PreflightSecret: []byte("01234567890123456789012345678901"),
	})
	if err != nil {
		t.Fatalf("create site service: %v", err)
	}
	repository := model.NewSiteRepository(database)

	authorized := createCorePendingSite(t, database, now)
	existingToken := "A04-existing-token"
	result, err := sites.Authorize(context.Background(), authorized.ID, dto.SiteAuthorizeRequest{
		Mode: "existing_token", RootUserID: stringPointerForCore("1"), AccessToken: stringPointerForCore(existingToken),
	}, "a04-authorize")
	if err != nil || result.RootUserID != "1" || len(factory.authTokens) != 1 || factory.authTokens[0] != existingToken {
		t.Fatalf("A04 existing-token authorization = %#v, tokens=%#v, err=%v", result, factory.authTokens, err)
	}
	persisted, err := repository.FindByID(context.Background(), authorized.ID)
	if err != nil || persisted.AccessTokenEncrypted == nil || *persisted.AccessTokenEncrypted == existingToken ||
		persisted.RootCreatedAt == nil || persisted.StatisticsStartAt == nil || persisted.AuthStatus != constant.SiteAuthAuthorized {
		t.Fatalf("A04 persisted authorization = %#v, err=%v", persisted, err)
	}
	initialRootCreatedAt := *persisted.RootCreatedAt
	initialStartAt := *persisted.StatisticsStartAt
	initialToken := *persisted.AccessTokenEncrypted

	client.statusErr = nil
	recovered, err := repository.FindByID(context.Background(), authorized.ID)
	if err != nil || recovered.AuthStatus != constant.SiteAuthAuthorized || recovered.StatisticsStatus != constant.SiteStatisticsBackfilling {
		t.Fatalf("A86 recovered active site = %#v, err=%v", recovered, err)
	}

	client.channels = dto.UpstreamChannelSnapshot{Total: 1, Items: []dto.UpstreamChannel{{ID: 1, Name: "renamed-channel", Status: 1}}}
	if _, err := sites.RecheckCapabilities(context.Background(), authorized.ID, "a81-channel-rename"); err != nil {
		t.Fatalf("A81 channel rename recheck: %v", err)
	}
	var channel model.SiteChannel
	if err := database.Where("site_id = ? AND remote_channel_id = ?", authorized.ID, 1).Take(&channel).Error; err != nil || channel.Name != "renamed-channel" || channel.RemoteMissing {
		t.Fatalf("A81 channel rename = %#v, err=%v", channel, err)
	}
	client.channels = dto.UpstreamChannelSnapshot{Total: 0, Items: []dto.UpstreamChannel{}}
	if _, err := sites.RecheckCapabilities(context.Background(), authorized.ID, "a81-channel-missing"); err != nil {
		t.Fatalf("A81 channel missing recheck: %v", err)
	}
	if err := database.Where("site_id = ? AND remote_channel_id = ?", authorized.ID, 1).Take(&channel).Error; err != nil || !channel.RemoteMissing || channel.Name != "renamed-channel" {
		t.Fatalf("A81 channel disappearance = %#v, err=%v", channel, err)
	}

	customer := createCoreCustomer(t, database, now, dto.CustomerStatusUsing)
	account := createCoreAccount(t, database, authorized.ID, customer.ID, 99, now)
	client.snapshotErr = service.ErrUpstreamUnavailable
	if _, err := sites.RecheckCapabilities(context.Background(), authorized.ID, "a10-pagination-failure"); !errors.Is(err, service.ErrUpstreamUnavailable) {
		t.Fatalf("A10 recheck failure = %v", err)
	}
	unchanged, err := model.NewAccountRepository(database).FindByID(context.Background(), account.ID)
	if err != nil || unchanged.RemoteState != model.AccountRemoteStateNormal || unchanged.RemoteMissingCount != 0 {
		t.Fatalf("A10 incomplete user page mutated account = %#v, err=%v", unchanged, err)
	}
	client.snapshotErr = nil

	client.root.CreatedAt = initialRootCreatedAt + 3600
	client.users[client.root.ID] = client.root
	client.snapshot = dto.UpstreamUserSnapshot{Total: 1, Items: []dto.UpstreamUser{client.root}}
	if _, err := sites.RecheckCapabilities(context.Background(), authorized.ID, "a24-root-created-at-changed"); !errors.Is(err, service.ErrSiteIncompatible) {
		t.Fatalf("A24 changed root created_at error = %v", err)
	}
	afterMismatch, err := repository.FindByID(context.Background(), authorized.ID)
	if err != nil || afterMismatch.RootCreatedAt == nil || *afterMismatch.RootCreatedAt != initialRootCreatedAt ||
		afterMismatch.StatisticsStartAt == nil || *afterMismatch.StatisticsStartAt != initialStartAt || afterMismatch.AccessTokenEncrypted == nil || *afterMismatch.AccessTokenEncrypted != initialToken {
		t.Fatalf("A24 changed root overwrote immutable authorization boundary: %#v err=%v", afterMismatch, err)
	}

	client.root.CreatedAt = initialRootCreatedAt
	client.users[client.root.ID] = client.root
	client.snapshot = dto.UpstreamUserSnapshot{Total: 1, Items: []dto.UpstreamUser{client.root}}
	if _, err := sites.RecheckCapabilities(context.Background(), authorized.ID, "a24-recovered-proof"); err != nil {
		t.Fatalf("A24 recovered proof recheck: %v", err)
	}
	refresh, err := sites.QueueRefresh(context.Background(), []int64{authorized.ID}, "a36-refresh")
	if err != nil || len(refresh) != 3 {
		t.Fatalf("A36 refresh queue = %#v, %v", refresh, err)
	}
	for _, run := range refresh {
		if run.TaskType != constant.TaskTypeSiteProbe && run.TaskType != constant.TaskTypeRealtimeStat && run.TaskType != constant.TaskTypeResourceSnapshot {
			t.Fatalf("A36 queued unexpected task type: %#v", run)
		}
	}
	duplicateRefresh, err := sites.QueueRefresh(context.Background(), []int64{authorized.ID}, "a37-refresh-duplicate")
	if err != nil || len(duplicateRefresh) != 3 {
		t.Fatalf("A37 duplicate refresh = %#v, %v", duplicateRefresh, err)
	}
	for _, run := range duplicateRefresh {
		if !run.Deduplicated || run.ID == "" {
			t.Fatalf("A37 duplicate task was not a successful deduplication: %#v", run)
		}
	}

	beforeDisable, err := repository.FindByID(context.Background(), authorized.ID)
	if err != nil {
		t.Fatalf("read A37 site before disable: %v", err)
	}
	if _, err := sites.Disable(context.Background(), authorized.ID); err != nil {
		t.Fatalf("A37 disable: %v", err)
	}
	afterDisable, err := repository.FindByID(context.Background(), authorized.ID)
	if err != nil || afterDisable.ManagementStatus != constant.SiteManagementDisabled || afterDisable.StatisticsStatus != constant.SiteStatisticsPaused || afterDisable.ConfigVersion <= beforeDisable.ConfigVersion {
		t.Fatalf("A37 first disable = %#v, err=%v", afterDisable, err)
	}
	if _, err := sites.Disable(context.Background(), authorized.ID); err != nil {
		t.Fatalf("A37 repeated disable: %v", err)
	}
	repeatedDisable, err := repository.FindByID(context.Background(), authorized.ID)
	if err != nil || repeatedDisable.ConfigVersion != afterDisable.ConfigVersion {
		t.Fatalf("A37 repeated disable changed fence = %#v, err=%v", repeatedDisable, err)
	}
	if _, err := sites.Enable(context.Background(), authorized.ID, "a37-enable"); err != nil {
		t.Fatalf("A37 enable: %v", err)
	}

	preflight, err := sites.PreflightBaseURL(context.Background(), authorized.ID, "https://new-origin.example.test", "a57-preflight")
	if err != nil || preflight.ChangeType != "origin" || preflight.PreflightToken == "" {
		t.Fatalf("A57 preflight = %#v, %v", preflight, err)
	}
	tokensBeforeUpdate := len(factory.authTokens)
	if _, err := sites.Update(context.Background(), authorized.ID, dto.SiteUpdateRequest{
		Name: repeatedDisable.Name, BaseURL: preflight.NormalizedBaseURL, Remark: repeatedDisable.Remark,
		BaseURLPreflightToken: preflight.PreflightToken, ConfirmSameSite: true,
	}); err != nil {
		t.Fatalf("A06/A57 origin update: %v", err)
	}
	updated, err := repository.FindByID(context.Background(), authorized.ID)
	if err != nil || updated.BaseURL != preflight.NormalizedBaseURL || updated.AuthStatus != constant.SiteAuthUnauthorized ||
		updated.AccessTokenEncrypted != nil || updated.ConfigVersion <= repeatedDisable.ConfigVersion || len(factory.authTokens) != tokensBeforeUpdate {
		t.Fatalf("A06/A57 origin update contract = %#v tokens=%#v err=%v", updated, factory.authTokens, err)
	}

	incompatible := createCorePendingSite(t, database, now)
	client.root.Role = 99
	client.users[client.root.ID] = client.root
	client.snapshot = dto.UpstreamUserSnapshot{Total: 1, Items: []dto.UpstreamUser{client.root}}
	if _, err := sites.Authorize(context.Background(), incompatible.ID, dto.SiteAuthorizeRequest{
		Mode: "existing_token", RootUserID: stringPointerForCore("1"), AccessToken: stringPointerForCore("A07-token"),
	}, "a07-invalid-root"); !errors.Is(err, service.ErrSiteIncompatible) {
		t.Fatalf("A07/A56 invalid root authorization error = %v", err)
	}
	failed, err := repository.FindByID(context.Background(), incompatible.ID)
	if err != nil || failed.AccessTokenEncrypted != nil || failed.RootUserID != nil || failed.RootCreatedAt != nil || failed.StatisticsStartAt != nil {
		t.Fatalf("A07/A56 persisted an invalid root authorization: %#v err=%v", failed, err)
	}
}

func stringPointerForCore(value string) *string {
	return &value
}

func TestA86DisabledRecheckStaysPausedWithoutBackfill(t *testing.T) {
	database := openCoreAcceptanceTransaction(t)
	now := int64(1_768_622_400)
	clock := testsupport.NewFakeClock(time.Unix(now, 0))
	cipher := newCoreCipher(t)
	client := newCoreSiteClient(now)
	factory := &coreSiteClientFactory{client: client}
	sites, err := service.NewSiteService(service.SiteServiceOptions{
		Repository: model.NewSiteRepository(database), ClientFactory: factory, Cipher: cipher, Clock: clock,
		PreflightSecret: []byte("01234567890123456789012345678901"),
	})
	if err != nil {
		t.Fatalf("create disabled recheck site service: %v", err)
	}
	site := createCoreAuthorizedSite(t, database, cipher, now)
	pausedAt := coreFloorHour(now)
	if err := database.Model(&model.Site{}).Where("id = ?", site.ID).Updates(map[string]any{
		"management_status": constant.SiteManagementDisabled, "statistics_status": constant.SiteStatisticsPaused,
		"disabled_at": pausedAt, "statistics_end_at": pausedAt,
	}).Error; err != nil {
		t.Fatalf("disable core recheck site: %v", err)
	}
	client.statusErr = nil
	result, err := sites.RecheckCapabilities(context.Background(), site.ID, "a86-disabled-failed")
	if err != nil || result.BackfillRunID != nil {
		t.Fatalf("A86 disabled failed recheck = %#v, %v", result, err)
	}
	persisted, err := model.NewSiteRepository(database).FindByID(context.Background(), site.ID)
	if err != nil || persisted.ManagementStatus != constant.SiteManagementDisabled || persisted.StatisticsStatus != constant.SiteStatisticsPaused || persisted.StatisticsEndAt == nil || *persisted.StatisticsEndAt != pausedAt {
		t.Fatalf("A86 disabled recheck did not remain paused = %#v, %v", persisted, err)
	}
	if strconv.FormatInt(*persisted.RootUserID, 10) != "1" || persisted.AccessTokenEncrypted == nil {
		t.Fatalf("A86 disabled recheck lost stored authorization")
	}
}
