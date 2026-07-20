package worker

import (
	"testing"

	"new-api-pilot/constant"
	"new-api-pilot/model"
)

func TestSchedulerSiteEligibleRejectsDisabledOrEndedProbe(t *testing.T) {
	site := model.Site{
		ID:               1,
		ConfigVersion:    2,
		ManagementStatus: constant.SiteManagementActive,
		AuthStatus:       constant.SiteAuthUnauthorized,
		OnlineStatus:     constant.SiteOnlineOffline,
	}
	if !schedulerSiteEligible(site, constant.TaskTypeSiteProbe) {
		t.Fatal("active site must remain probe eligible before authorization and while offline")
	}

	endedAt := int64(1_752_400_800)
	tests := []struct {
		name   string
		mutate func(*model.Site)
	}{
		{name: "missing id", mutate: func(site *model.Site) { site.ID = 0 }},
		{name: "missing config version", mutate: func(site *model.Site) { site.ConfigVersion = 0 }},
		{name: "management disabled", mutate: func(site *model.Site) { site.ManagementStatus = constant.SiteManagementDisabled }},
		{name: "statistics ended", mutate: func(site *model.Site) { site.StatisticsEndAt = &endedAt }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			current := site
			test.mutate(&current)
			if schedulerSiteEligible(current, constant.TaskTypeSiteProbe) {
				t.Fatal("site unexpectedly eligible for probe")
			}
		})
	}
}
