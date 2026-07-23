package service

import (
	"context"
	"net/netip"
	"time"

	"new-api-pilot/dto"
)

type SiteUpstreamClient interface {
	Status(context.Context, string) (dto.UpstreamStatus, error)
	Self(context.Context, string) (dto.UpstreamIdentity, error)
	GetUser(context.Context, string, int64) (dto.UpstreamUser, error)
	SnapshotUsers(context.Context, string) (dto.UpstreamUserSnapshot, error)
	SnapshotChannels(context.Context, string) (dto.UpstreamChannelSnapshot, error)
	FlowHour(context.Context, string, int64) ([]dto.UpstreamFlowRow, error)
	DataHour(context.Context, string, int64) ([]dto.UpstreamDataRow, error)
	Instances(context.Context, string) ([]dto.UpstreamInstance, error)
	LogStat(context.Context, string) (dto.UpstreamLogStat, error)
	PerformanceSummary(context.Context, string, int) (dto.UpstreamPerformanceSummary, error)
	LoginAndGenerateAccessToken(context.Context, string, string, string) (dto.UpstreamIdentity, string, error)
	CloseIdleConnections()
}

type SiteRemoteUserClient interface {
	SiteUpstreamClient
	ListUsersPage(context.Context, string, int) (dto.UpstreamUserPage, error)
	SearchUsers(context.Context, string, string, int) (dto.UpstreamUserPage, error)
}

type SiteClientFactory interface {
	NewPublic(baseURL string) (SiteUpstreamClient, error)
	NewAuthenticated(baseURL, credentialOrigin, accessToken string, rootUserID int64) (SiteUpstreamClient, error)
}

type SiteClientFactoryOptions struct {
	Runtime             *RuntimeSettingsStore
	AllowedHostSuffixes []string
	AllowedCIDRs        []netip.Prefix
	CAFile              string
	ConnectTimeout      time.Duration
	HeaderTimeout       time.Duration
	RequestTimeout      time.Duration
	ExportTimeout       time.Duration
	Metrics             UpstreamMetricsRecorder
	Governor            UpstreamGovernor
}

type ConfiguredSiteClientFactory struct {
	options SiteClientFactoryOptions
}

func NewConfiguredSiteClientFactory(options SiteClientFactoryOptions) *ConfiguredSiteClientFactory {
	return &ConfiguredSiteClientFactory{options: options}
}

func (factory *ConfiguredSiteClientFactory) NewPublic(baseURL string) (SiteUpstreamClient, error) {
	return factory.newClient(NewAPIClientOptions{BaseURL: baseURL})
}

func (factory *ConfiguredSiteClientFactory) NewAuthenticated(baseURL, credentialOrigin, accessToken string, rootUserID int64) (SiteUpstreamClient, error) {
	return factory.newClient(NewAPIClientOptions{
		BaseURL: baseURL, CredentialOrigin: credentialOrigin, AccessToken: accessToken, RootUserID: rootUserID,
	})
}

func (factory *ConfiguredSiteClientFactory) newClient(options NewAPIClientOptions) (SiteUpstreamClient, error) {
	if factory.options.Runtime != nil {
		runtime := factory.options.Runtime.Snapshot()
		options.AllowedHostSuffixes = runtime.AllowedHosts
		options.AllowedCIDRs = runtime.AllowedCIDRs
		options.ConnectTimeout = runtime.ConnectTimeout
		options.HeaderTimeout = runtime.HeaderTimeout
		options.RequestTimeout = runtime.RequestTimeout
		options.ExportTimeout = runtime.ExportTimeout
		options.Governor = runtime.Governor
	} else {
		options.AllowedHostSuffixes = factory.options.AllowedHostSuffixes
		options.AllowedCIDRs = factory.options.AllowedCIDRs
		options.ConnectTimeout = factory.options.ConnectTimeout
		options.HeaderTimeout = factory.options.HeaderTimeout
		options.RequestTimeout = factory.options.RequestTimeout
		options.ExportTimeout = factory.options.ExportTimeout
		options.Governor = factory.options.Governor
	}
	options.CAFile = factory.options.CAFile
	options.Metrics = factory.options.Metrics
	return NewNewAPIClient(options)
}
