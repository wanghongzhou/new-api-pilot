package integration_test

import (
	"context"

	"new-api-pilot/dto"
	"new-api-pilot/service"
)

// collectionSiteClient keeps collection-specific upstream behavior out of the
// shared core acceptance harness, which is also used by unrelated domains.
type collectionSiteClient struct {
	*coreSiteClient
	flow      []dto.UpstreamFlowRow
	data      []dto.UpstreamDataRow
	flowErr   error
	dataErr   error
	flowCalls int
	dataCalls int
}

func newCollectionSiteClient(now int64) *collectionSiteClient {
	return &collectionSiteClient{coreSiteClient: newCoreSiteClient(now)}
}

func (client *collectionSiteClient) FlowHour(context.Context, string, int64) ([]dto.UpstreamFlowRow, error) {
	client.flowCalls++
	return append([]dto.UpstreamFlowRow(nil), client.flow...), client.flowErr
}

func (client *collectionSiteClient) DataHour(context.Context, string, int64) ([]dto.UpstreamDataRow, error) {
	client.dataCalls++
	return append([]dto.UpstreamDataRow(nil), client.data...), client.dataErr
}

type collectionSiteClientFactory struct {
	client *collectionSiteClient
}

func (factory *collectionSiteClientFactory) NewPublic(string) (service.SiteUpstreamClient, error) {
	return factory.client, nil
}

func (factory *collectionSiteClientFactory) NewAuthenticated(string, string, string, int64) (service.SiteUpstreamClient, error) {
	return factory.client, nil
}
