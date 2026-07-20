package service

import "new-api-pilot/model"

func bindUsageAggregationCommit(
	request UsageCollectionRequest,
	now int64,
	newFacts []model.UsageFactInput,
	factMutation model.UsageFactMutation,
) (model.UsageAggregationCommit, error) {
	return model.NewUsageAggregationCommit(model.UsageAggregationMutationRequest{
		RunID: request.Run.ID, WindowID: request.Window.ID, SiteID: request.Window.SiteID,
		ExpectedConfigVersion: request.Run.SiteConfigVersion, HourTS: request.Window.HourTS,
		AttemptCount: request.Window.AttemptCount, RequestID: request.RequestID, Now: now,
		NewFacts: newFacts,
	}, factMutation)
}
