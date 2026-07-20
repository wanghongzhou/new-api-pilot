package service

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"new-api-pilot/common"
	"new-api-pilot/constant"
	"new-api-pilot/dto"
	"new-api-pilot/model"
)

type siteAuthorizationEvidence struct {
	Root           dto.UpstreamUser
	Snapshot       dto.UpstreamUserSnapshot
	Proof          dto.SiteFirstUserProof
	Status         dto.UpstreamStatus
	Capabilities   []dto.SiteCapabilityResult
	Channels       dto.UpstreamChannelSnapshot
	ChannelsValid  bool
	Instances      []dto.UpstreamInstance
	InstancesValid bool
	Realtime       dto.UpstreamLogStat
	RealtimeValid  bool
	FlowValidation string
}

func (service *SiteService) Authorize(ctx context.Context, siteID int64, request dto.SiteAuthorizeRequest, requestID string) (dto.SiteAuthorizationResult, error) {
	site, err := service.sites.FindByID(ctx, siteID)
	if err != nil {
		if model.IsNotFound(err) {
			return dto.SiteAuthorizationResult{}, ErrSiteNotFound
		}
		return dto.SiteAuthorizationResult{}, err
	}
	rootUserID, accessToken, err := service.obtainAuthorizationCredentials(ctx, site, request, requestID)
	if err != nil {
		return dto.SiteAuthorizationResult{}, err
	}
	client, err := service.clients.NewAuthenticated(site.BaseURL, site.BaseURL, accessToken, rootUserID)
	if err != nil {
		return dto.SiteAuthorizationResult{}, err
	}
	defer client.CloseIdleConnections()
	evidence, err := service.collectAuthorizationEvidence(ctx, client, site, rootUserID, requestID)
	if err != nil {
		return dto.SiteAuthorizationResult{}, err
	}
	return service.commitAuthorization(ctx, site, accessToken, evidence, requestID, true)
}

func (service *SiteService) RecheckCapabilities(ctx context.Context, siteID int64, requestID string) (dto.SiteAuthorizationResult, error) {
	site, err := service.sites.FindByID(ctx, siteID)
	if err != nil {
		if model.IsNotFound(err) {
			return dto.SiteAuthorizationResult{}, ErrSiteNotFound
		}
		return dto.SiteAuthorizationResult{}, err
	}
	if site.RootUserID == nil || site.AccessTokenEncrypted == nil || site.RootCreatedAt == nil || site.StatisticsStartAt == nil {
		return dto.SiteAuthorizationResult{}, ErrSiteInvalidState
	}
	plaintext, err := service.cipher.Decrypt(*site.AccessTokenEncrypted, siteTokenAAD(site.ID))
	if err != nil {
		if markErr := service.markRecheckIdentityFailure(ctx, site, true); markErr != nil {
			return dto.SiteAuthorizationResult{}, markErr
		}
		return dto.SiteAuthorizationResult{}, ErrSiteIncompatible
	}
	accessToken := string(plaintext)
	client, err := service.clients.NewAuthenticated(site.BaseURL, site.BaseURL, accessToken, *site.RootUserID)
	if err != nil {
		return dto.SiteAuthorizationResult{}, err
	}
	defer client.CloseIdleConnections()
	evidence, err := service.collectAuthorizationEvidence(ctx, client, site, *site.RootUserID, requestID)
	if err != nil {
		if errors.Is(err, ErrSiteIncompatible) || errors.Is(err, ErrUpstreamAuthExpired) {
			if markErr := service.markRecheckIdentityFailure(ctx, site, errors.Is(err, ErrUpstreamAuthExpired)); markErr != nil {
				return dto.SiteAuthorizationResult{}, markErr
			}
		}
		return dto.SiteAuthorizationResult{}, err
	}
	return service.commitAuthorization(ctx, site, accessToken, evidence, requestID, false)
}

func (service *SiteService) obtainAuthorizationCredentials(ctx context.Context, site model.Site, request dto.SiteAuthorizeRequest, requestID string) (int64, string, error) {
	if request.Mode == "existing_token" {
		rootUserID, ok := request.ParsedRootUserID()
		if !ok || request.AccessToken == nil {
			return 0, "", ErrSiteIncompatible
		}
		return rootUserID, *request.AccessToken, nil
	}
	if request.Username == nil || request.Password == nil {
		return 0, "", ErrSiteIncompatible
	}
	client, err := service.clients.NewPublic(site.BaseURL)
	if err != nil {
		return 0, "", err
	}
	defer client.CloseIdleConnections()
	identity, token, err := client.LoginAndGenerateAccessToken(ctx, requestID, *request.Username, *request.Password)
	if err != nil {
		return 0, "", err
	}
	return identity.ID, token, nil
}

func (service *SiteService) collectAuthorizationEvidence(ctx context.Context, client SiteUpstreamClient, site model.Site, rootUserID int64, requestID string) (siteAuthorizationEvidence, error) {
	if _, err := client.Self(ctx, requestID); err != nil {
		return siteAuthorizationEvidence{}, siteIdentityError(err)
	}
	root, err := client.GetUser(ctx, requestID, rootUserID)
	if err != nil {
		return siteAuthorizationEvidence{}, siteIdentityError(err)
	}
	if root.CreatedAt > service.clock.Now().Unix() {
		return siteAuthorizationEvidence{}, ErrSiteIncompatible
	}
	snapshot, err := client.SnapshotUsers(ctx, requestID)
	if err != nil {
		return siteAuthorizationEvidence{}, siteIdentityError(err)
	}
	proof, err := proveFirstUser(root, snapshot)
	if err != nil {
		return siteAuthorizationEvidence{}, err
	}
	if site.RootUserID != nil && (*site.RootUserID != root.ID || site.RootCreatedAt == nil || *site.RootCreatedAt != root.CreatedAt) {
		return siteAuthorizationEvidence{}, ErrSiteIncompatible
	}
	evidence := siteAuthorizationEvidence{Root: root, Snapshot: snapshot, Proof: proof, FlowValidation: constant.CapabilityStatusSkipped}
	service.collectCapabilityEvidence(ctx, client, site, requestID, &evidence)
	return evidence, nil
}

func proveFirstUser(root dto.UpstreamUser, snapshot dto.UpstreamUserSnapshot) (dto.SiteFirstUserProof, error) {
	if root.ID <= 0 || root.Role != 100 || root.Status != 1 || root.Deleted || root.CreatedAt <= 0 || snapshot.Total <= 0 || int64(len(snapshot.Items)) != snapshot.Total {
		return dto.SiteFirstUserProof{}, ErrSiteIncompatible
	}
	minID := snapshot.Items[0].ID
	earliest := snapshot.Items[0].CreatedAt
	foundRoot := false
	for _, user := range snapshot.Items {
		if user.ID < minID {
			minID = user.ID
		}
		if user.CreatedAt < earliest {
			earliest = user.CreatedAt
		}
		if user.ID == root.ID {
			foundRoot = user.CreatedAt == root.CreatedAt && user.Role == 100 && user.Status == 1 && !user.Deleted
		}
	}
	if !foundRoot || minID != root.ID || root.CreatedAt != earliest {
		return dto.SiteFirstUserProof{}, ErrSiteIncompatible
	}
	return dto.SiteFirstUserProof{
		SnapshotTotal: snapshot.Total, MinUserID: strconv.FormatInt(minID, 10),
		EarliestCreatedAt: earliest, Passed: true,
	}, nil
}

func (service *SiteService) collectCapabilityEvidence(ctx context.Context, client SiteUpstreamClient, site model.Site, requestID string, evidence *siteAuthorizationEvidence) {
	results := make(map[string]dto.SiteCapabilityResult, len(constant.SiteCapabilityKeys()))
	for _, key := range []string{
		constant.CapabilitySelfIdentity, constant.CapabilityRootIdentity,
		constant.CapabilityFirstUserProof, constant.CapabilityUserPagination,
	} {
		results[key] = service.capabilityResult(site.ID, key, constant.CapabilityStatusPassed, nil, dto.UpstreamStatus{})
	}

	status, statusErr := client.Status(ctx, requestID)
	evidence.Status = status
	switch {
	case statusErr == nil:
		results[constant.CapabilityStatusContract] = service.capabilityResult(site.ID, constant.CapabilityStatusContract, constant.CapabilityStatusPassed, nil, status)
		results[constant.CapabilityDataExportEnabled] = service.capabilityResult(site.ID, constant.CapabilityDataExportEnabled, constant.CapabilityStatusPassed, nil, status)
	case errors.Is(statusErr, ErrUpstreamExportDisabled):
		results[constant.CapabilityStatusContract] = service.capabilityResult(site.ID, constant.CapabilityStatusContract, constant.CapabilityStatusPassed, nil, status)
		results[constant.CapabilityDataExportEnabled] = service.capabilityResult(site.ID, constant.CapabilityDataExportEnabled, constant.CapabilityStatusFailed, statusErr, status)
	default:
		for _, key := range []string{constant.CapabilityStatusContract, constant.CapabilityDataExportEnabled} {
			results[key] = service.capabilityResult(site.ID, key, constant.CapabilityStatusFailed, statusErr, status)
		}
	}

	channels, err := client.SnapshotChannels(ctx, requestID)
	if err == nil {
		evidence.Channels, evidence.ChannelsValid = channels, true
		results[constant.CapabilityChannelPagination] = service.capabilityResult(site.ID, constant.CapabilityChannelPagination, constant.CapabilityStatusPassed, nil, status)
	} else {
		results[constant.CapabilityChannelPagination] = service.capabilityResult(site.ID, constant.CapabilityChannelPagination, constant.CapabilityStatusFailed, err, status)
	}

	flowStatus, dataStatus, consistencyStatus := service.checkFlowDataCapabilities(ctx, client, requestID)
	results[constant.CapabilityFlowContract] = service.capabilityResult(site.ID, constant.CapabilityFlowContract, flowStatus.status, flowStatus.err, status)
	results[constant.CapabilityDataContract] = service.capabilityResult(site.ID, constant.CapabilityDataContract, dataStatus.status, dataStatus.err, status)
	results[constant.CapabilityFlowDataConsistency] = service.capabilityResult(site.ID, constant.CapabilityFlowDataConsistency, consistencyStatus.status, consistencyStatus.err, status)
	evidence.FlowValidation = consistencyStatus.status

	instances, err := client.Instances(ctx, requestID)
	if err == nil {
		evidence.Instances, evidence.InstancesValid = instances, true
		results[constant.CapabilityInstanceContract] = service.capabilityResult(site.ID, constant.CapabilityInstanceContract, constant.CapabilityStatusPassed, nil, status)
	} else {
		results[constant.CapabilityInstanceContract] = service.capabilityResult(site.ID, constant.CapabilityInstanceContract, constant.CapabilityStatusFailed, err, status)
	}

	realtime, err := client.LogStat(ctx, requestID)
	if err == nil {
		evidence.Realtime, evidence.RealtimeValid = realtime, true
		results[constant.CapabilityRealtimeContract] = service.capabilityResult(site.ID, constant.CapabilityRealtimeContract, constant.CapabilityStatusPassed, nil, status)
	} else {
		results[constant.CapabilityRealtimeContract] = service.capabilityResult(site.ID, constant.CapabilityRealtimeContract, constant.CapabilityStatusFailed, err, status)
	}

	evidence.Capabilities = make([]dto.SiteCapabilityResult, 0, len(results))
	for _, key := range constant.SiteCapabilityKeys() {
		evidence.Capabilities = append(evidence.Capabilities, results[key])
	}
}

type capabilityCheck struct {
	status string
	err    error
}

func (service *SiteService) checkFlowDataCapabilities(ctx context.Context, client SiteUpstreamClient, requestID string) (capabilityCheck, capabilityCheck, capabilityCheck) {
	flowCheck := capabilityCheck{status: constant.CapabilityStatusPassed}
	dataCheck := capabilityCheck{status: constant.CapabilityStatusPassed}
	consistency := capabilityCheck{status: constant.CapabilityStatusSkipped}
	latestHour := floorHour(service.clock.Now().Unix()) - 3600
	for offset := int64(0); offset < 24; offset++ {
		hour := latestHour - offset*3600
		if hour <= 0 {
			break
		}
		flow, flowErr := client.FlowHour(ctx, requestID, hour)
		data, dataErr := client.DataHour(ctx, requestID, hour)
		if flowErr != nil {
			flowCheck = capabilityCheck{status: constant.CapabilityStatusFailed, err: flowErr}
		}
		if dataErr != nil {
			dataCheck = capabilityCheck{status: constant.CapabilityStatusFailed, err: dataErr}
		}
		if flowErr != nil || dataErr != nil {
			return flowCheck, dataCheck, capabilityCheck{status: constant.CapabilityStatusFailed, err: firstError(flowErr, dataErr)}
		}
		if len(flow) == 0 && len(data) == 0 {
			continue
		}
		if err := ValidateFlowDataConsistency(flow, data); err != nil {
			return flowCheck, dataCheck, capabilityCheck{status: constant.CapabilityStatusFailed, err: err}
		}
		return flowCheck, dataCheck, capabilityCheck{status: constant.CapabilityStatusPassed}
	}
	return flowCheck, dataCheck, consistency
}

func (service *SiteService) capabilityResult(siteID int64, key, status string, cause error, upstreamStatus dto.UpstreamStatus) dto.SiteCapabilityResult {
	code := constant.MessageCapabilityOK
	params := map[string]any{"site_id": strconv.FormatInt(siteID, 10), "capability_key": key}
	if status == constant.CapabilityStatusSkipped {
		code = constant.MessageCapabilityNoTrafficSkipped
	} else if status == constant.CapabilityStatusFailed {
		switch {
		case key == constant.CapabilityDataExportEnabled && errors.Is(cause, ErrUpstreamExportDisabled):
			code = constant.MessageCapabilityExportDisabled
		case key == constant.CapabilitySelfIdentity || key == constant.CapabilityRootIdentity:
			code = constant.MessageCapabilityIdentityFailed
		case key == constant.CapabilityFirstUserProof:
			code = constant.MessageCapabilityFirstUserProofFailed
		case errors.Is(cause, ErrUpstreamUnavailable) || errors.Is(cause, ErrUpstreamRateLimited) || errors.Is(cause, ErrUpstreamRemote):
			code = constant.MessageCapabilityUpstreamUnavailable
		default:
			code = constant.MessageCapabilityResponseInvalid
		}
	}
	message, err := dto.NewMessageRef(code, params, "")
	if err != nil {
		message = dto.MustMessageRef(constant.MessageInternalContractError, map[string]any{"component": "site_capability"}, "")
	}
	return dto.SiteCapabilityResult{Key: key, Status: status, Message: message}
}

func (service *SiteService) commitAuthorization(ctx context.Context, original model.Site, accessToken string, evidence siteAuthorizationEvidence, requestID string, rotateCredentials bool) (dto.SiteAuthorizationResult, error) {
	encrypted, err := service.cipher.Encrypt([]byte(accessToken), siteTokenAAD(original.ID))
	if err != nil {
		return dto.SiteAuthorizationResult{}, fmt.Errorf("encrypt site token: %w", err)
	}
	var backfillRunID *string
	committedAt := int64(0)
	userSnapshotApplied := false
	userObservations := siteUserObservations(evidence.Snapshot)
	err = service.sites.WithTransaction(ctx, func(repository *model.SiteRepository) error {
		site, lockErr := repository.FindByIDForUpdate(ctx, original.ID)
		if lockErr != nil {
			if model.IsNotFound(lockErr) {
				return ErrSiteNotFound
			}
			return lockErr
		}
		if site.ConfigVersion != original.ConfigVersion || site.BaseURL != original.BaseURL {
			return ErrSiteConfigChanged
		}
		if site.RootUserID != nil && (*site.RootUserID != evidence.Root.ID || site.RootCreatedAt == nil || *site.RootCreatedAt != evidence.Root.CreatedAt) {
			return ErrSiteIncompatible
		}
		committedAt = monotonicMutationTime(service.clock.Now().Unix(), site.UpdatedAt)
		oldReady, err := requiredCapabilitiesReady(ctx, repository, site.ID)
		if err != nil {
			return err
		}
		newReady := authorizationCapabilitiesReady(evidence.Capabilities)
		needsFence := rotateCredentials || site.AuthStatus != constant.SiteAuthAuthorized || oldReady != newReady
		if needsFence {
			if err := repository.BumpSiteFence(ctx, &site, committedAt); err != nil {
				return err
			}
		}
		oldStatisticsStatus := site.StatisticsStatus
		rootID := evidence.Root.ID
		rootCreatedAt := evidence.Root.CreatedAt
		statisticsStartAt := floorHour(rootCreatedAt)
		source := "root_created_at"
		if site.RootUserID == nil {
			site.RootUserID = &rootID
			site.RootCreatedAt = &rootCreatedAt
			site.StatisticsStartAt = &statisticsStartAt
			site.StatisticsStartSource = &source
		}
		if rotateCredentials {
			site.AccessTokenEncrypted = &encrypted
		}
		site.AuthStatus = constant.SiteAuthAuthorized
		applyAuthorizationStatus(&site, evidence, committedAt)
		shouldStartRun := false
		if site.ManagementStatus == constant.SiteManagementDisabled || site.StatisticsEndAt != nil {
			site.StatisticsStatus = constant.SiteStatisticsPaused
		} else if newReady {
			shouldStartRun = rotateCredentials || oldStatisticsStatus == constant.SiteStatisticsPendingConfig || oldStatisticsStatus == constant.SiteStatisticsError
			if shouldStartRun {
				site.StatisticsStatus = constant.SiteStatisticsBackfilling
			}
		} else if authorizationConfigurationFailure(evidence.Capabilities) {
			site.StatisticsStatus = constant.SiteStatisticsPendingConfig
		} else {
			site.StatisticsStatus = constant.SiteStatisticsError
		}
		capabilities, err := capabilityModels(site.ID, evidence.Capabilities, committedAt)
		if err != nil {
			return err
		}
		if err := repository.ReplaceCapabilities(ctx, site.ID, capabilities); err != nil {
			return err
		}
		if evidence.ChannelsValid {
			if err := repository.SyncChannels(ctx, site.ID, committedAt, channelModels(evidence.Channels)); err != nil {
				return err
			}
		}
		if evidence.InstancesValid {
			if site.MonitoringStartAt == nil {
				value := floorMinute(committedAt)
				site.MonitoringStartAt = &value
			}
			nodeNames := make([]string, 0, len(evidence.Instances))
			for _, instance := range evidence.Instances {
				nodeNames = append(nodeNames, instance.NodeName)
			}
			if err := repository.RetireMissingInstances(ctx, site.ID, committedAt, nodeNames); err != nil {
				return err
			}
			if err := repository.SyncInstances(ctx, instanceModels(site.ID, committedAt, evidence.Instances)); err != nil {
				return err
			}
		}
		site.UpdatedAt = committedAt
		if err := repository.Save(ctx, &site); err != nil {
			return err
		}
		if _, err := repository.ApplyAuthorizationSiteUserSnapshot(
			ctx, site, committedAt, floorHour(committedAt), userObservations,
		); err != nil {
			return err
		}
		userSnapshotApplied = true
		if shouldStartRun {
			run, err := service.enqueueInitialBackfill(ctx, repository, site, requestID)
			if err != nil {
				return err
			}
			value := strconv.FormatInt(run.ID, 10)
			backfillRunID = &value
			if run.Status == "success" {
				site.StatisticsStatus = constant.SiteStatisticsReady
				site.UpdatedAt = committedAt
				if err := repository.Save(ctx, &site); err != nil {
					return err
				}
			}
		}
		return nil
	})
	if err != nil {
		return dto.SiteAuthorizationResult{}, err
	}
	if service.postCommit != nil {
		service.postCommit.NotifyAfterCommit(ctx, AlertPostCommitTrigger{
			Source: AlertSampleSourceAuth, SiteID: original.ID, ObservedAt: committedAt,
		})
		if userSnapshotApplied {
			service.postCommit.NotifyAfterCommit(ctx, AlertPostCommitTrigger{
				Source: AlertSampleSourceUser, SiteID: original.ID, ObservedAt: committedAt,
			})
		}
	}
	// Probe only after the authorization transaction has committed. A failed
	// probe is persisted by probeWithSnapshot and must not undo credentials or
	// prevent the independently queued backfill from running.
	_, _ = service.Probe(ctx, original.ID, requestID+"_immediate_probe")
	version := evidence.Status.Version
	systemName := evidence.Status.SystemName
	exportEnabled := evidence.Status.DataExportEnabled
	return dto.SiteAuthorizationResult{
		RootUserID: strconv.FormatInt(evidence.Root.ID, 10), Version: &version, SystemName: &systemName,
		DataExportEnabled: &exportEnabled, FirstUserProof: evidence.Proof,
		Capabilities: evidence.Capabilities, FlowDataValidation: evidence.FlowValidation,
		RootCreatedAt: evidence.Root.CreatedAt, StatisticsStartAt: floorHour(evidence.Root.CreatedAt),
		BackfillRunID: backfillRunID,
	}, nil
}

func applyAuthorizationStatus(site *model.Site, evidence siteAuthorizationEvidence, now int64) {
	if evidence.Status.Version != "" {
		site.Version = evidence.Status.Version
		site.SystemName = evidence.Status.SystemName
		site.DataExportEnabled = evidence.Status.DataExportEnabled
		quota := evidence.Status.QuotaPerUnit
		rate := evidence.Status.USDExchangeRate
		site.QuotaPerUnit = &quota
		site.USDExchangeRate = &rate
		site.LastRateAt = &now
	}
	if evidence.RealtimeValid {
		site.CurrentRPM = evidence.Realtime.RPM
		site.CurrentTPM = evidence.Realtime.TPM
		site.LastRealtimeStatAt = &now
	}
}

func capabilityModels(siteID int64, results []dto.SiteCapabilityResult, checkedAt int64) ([]model.SiteCapability, error) {
	models := make([]model.SiteCapability, 0, len(results))
	for _, result := range results {
		params, err := common.Marshal(result.Message.Params)
		if err != nil {
			return nil, fmt.Errorf("encode capability params: %w", err)
		}
		models = append(models, model.SiteCapability{
			SiteID: siteID, CapabilityKey: result.Key, Status: result.Status,
			MessageCode: string(result.Message.Code), MessageParams: params, CheckedAt: checkedAt,
		})
	}
	return models, nil
}

func channelModels(snapshot dto.UpstreamChannelSnapshot) []model.SiteChannel {
	channels := make([]model.SiteChannel, 0, len(snapshot.Items))
	for _, channel := range snapshot.Items {
		balance := channel.Balance
		if balance == "" {
			balance = "0"
		}
		channels = append(channels, model.SiteChannel{RemoteChannelID: channel.ID, Name: channel.Name, RemoteType: channel.Type,
			RemoteStatus: channel.Status, TestTime: channel.TestTime, ResponseTimeMS: channel.ResponseTimeMS, Balance: balance,
			BalanceUpdatedAt: channel.BalanceUpdatedAt, Models: channel.Models, RemoteGroup: channel.Group, UsedQuota: channel.UsedQuota,
			Priority: channel.Priority, Weight: channel.Weight, AutoBan: channel.AutoBan, Tag: channel.Tag})
	}
	return channels
}

func instanceModels(siteID, now int64, instances []dto.UpstreamInstance) []model.SiteInstanceWrite {
	minute := floorMinute(now)
	writes := make([]model.SiteInstanceWrite, 0, len(instances))
	for _, instance := range instances {
		currentStatus := "stale"
		if instance.Status == "online" && instance.LastSeenAt > 0 && now-instance.LastSeenAt <= defaultInstanceStaleSeconds {
			currentStatus = "online"
		}
		isMaster := instance.IsMaster != nil && *instance.IsMaster
		startedAt := positiveTimestampPointer(instance.StartedAt)
		lastSeenAt := positiveTimestampPointer(instance.LastSeenAt)
		writes = append(writes, model.SiteInstanceWrite{
			Instance: model.SiteInstance{
				SiteID: siteID, NodeName: instance.NodeName, Hostname: instance.Hostname, IsMaster: isMaster,
				RuntimeVersion: instance.RuntimeVersion, GOOS: instance.GOOS, GOARCH: instance.GOARCH,
				UpstreamStatus: instance.Status, UpstreamStaleAfterSeconds: &instance.StaleAfterSeconds,
				CurrentStatus: currentStatus, FirstSeenAt: now, StartedAt: startedAt, LastSeenAt: lastSeenAt,
				LastSyncedAt: now, CreatedAt: now, UpdatedAt: now,
			},
			Sample: model.SiteInstanceStatusMinutely{
				SiteID: siteID, NodeName: instance.NodeName, MinuteTS: minute, Status: currentStatus,
				CPUPercent: instance.CPUPercent, MemoryPercent: instance.MemoryPercent,
				DiskUsedPercent: instance.StorageUsedPercent, DiskTotalBytes: instance.StorageTotalBytes,
				DiskUsedBytes: instance.StorageUsedBytes, StartedAt: startedAt, LastSeenAt: lastSeenAt, CreatedAt: now,
			},
		})
	}
	return writes
}

func authorizationCapabilitiesReady(capabilities []dto.SiteCapabilityResult) bool {
	if len(capabilities) != len(constant.SiteCapabilityKeys()) {
		return false
	}
	for _, capability := range capabilities {
		if capability.Status == constant.CapabilityStatusFailed ||
			(capability.Status == constant.CapabilityStatusSkipped && capability.Key != constant.CapabilityFlowDataConsistency) {
			return false
		}
	}
	return true
}

func authorizationConfigurationFailure(capabilities []dto.SiteCapabilityResult) bool {
	hasConfigurationFailure := false
	for _, capability := range capabilities {
		if capability.Status != constant.CapabilityStatusFailed {
			continue
		}
		if capability.Key == constant.CapabilityDataExportEnabled {
			hasConfigurationFailure = true
			continue
		}
		return false
	}
	return hasConfigurationFailure
}

func siteIdentityError(err error) error {
	if errors.Is(err, ErrUpstreamUnavailable) || errors.Is(err, ErrUpstreamRateLimited) ||
		errors.Is(err, ErrUpstreamRemote) || errors.Is(err, ErrUpstreamAddressForbidden) ||
		errors.Is(err, ErrUpstreamResponseTooLarge) || errors.Is(err, ErrUpstreamTokenRotationResultUnknown) {
		return err
	}
	return ErrSiteIncompatible
}

func (service *SiteService) markRecheckIdentityFailure(ctx context.Context, original model.Site, expired bool) error {
	committedAt := int64(0)
	err := service.sites.WithTransaction(ctx, func(repository *model.SiteRepository) error {
		site, err := repository.FindByIDForUpdate(ctx, original.ID)
		if err != nil {
			return err
		}
		if site.ConfigVersion != original.ConfigVersion || site.BaseURL != original.BaseURL {
			return ErrSiteConfigChanged
		}
		capabilities, err := repository.ListCapabilities(ctx, site.ID)
		if err != nil {
			return err
		}
		identityAlreadyFailed := false
		for _, capability := range capabilities {
			if capability.CapabilityKey == constant.CapabilitySelfIdentity && capability.Status == constant.CapabilityStatusFailed {
				identityAlreadyFailed = true
				break
			}
		}
		authChanged := expired && site.AuthStatus != constant.SiteAuthExpired
		now := service.clock.Now().Unix()
		committedAt = monotonicMutationTime(now, site.UpdatedAt)
		if authChanged || !identityAlreadyFailed {
			if err := repository.BumpSiteFence(ctx, &site, committedAt); err != nil {
				return err
			}
		}
		if expired {
			site.AuthStatus = constant.SiteAuthExpired
		}
		if site.ManagementStatus == constant.SiteManagementDisabled {
			site.StatisticsStatus = constant.SiteStatisticsPaused
		} else {
			site.StatisticsStatus = constant.SiteStatisticsError
		}
		var cause error = ErrSiteIncompatible
		if expired {
			cause = ErrUpstreamAuthExpired
		}
		result := service.capabilityResult(site.ID, constant.CapabilitySelfIdentity, constant.CapabilityStatusFailed, cause, dto.UpstreamStatus{})
		models, err := capabilityModels(site.ID, []dto.SiteCapabilityResult{result}, committedAt)
		if err != nil {
			return err
		}
		if err := repository.UpsertCapabilities(ctx, site.ID, models); err != nil {
			return err
		}
		site.UpdatedAt = committedAt
		return repository.Save(ctx, &site)
	})
	if err != nil {
		return err
	}
	if service.postCommit != nil {
		service.postCommit.NotifyAfterCommit(ctx, AlertPostCommitTrigger{
			Source: AlertSampleSourceAuth, SiteID: original.ID, ObservedAt: committedAt,
		})
	}
	return nil
}

func siteTokenAAD(siteID int64) string {
	return "site:" + strconv.FormatInt(siteID, 10) + ":access_token"
}

func positiveTimestampPointer(value int64) *int64 {
	if value <= 0 {
		return nil
	}
	result := value
	return &result
}

func firstError(first, second error) error {
	if first != nil {
		return first
	}
	return second
}
