package testsupport

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestFakeUpstreamServesSupportedScenario(t *testing.T) {
	upstream := NewFakeUpstream(t, "supported")

	response, err := upstream.Client().Get(upstream.URL() + "/api/status")
	if err != nil {
		t.Fatalf("GET status: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status code = %d, want 200", response.StatusCode)
	}

	request, err := http.NewRequest(http.MethodGet, upstream.URL()+"/api/user/?p=1&page_size=100", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "fixture-root-token")
	request.Header.Set("New-Api-User", "1")
	request.Header.Set("X-Request-ID", "fixture-request-1")
	response, err = upstream.Client().Do(request)
	if err != nil {
		t.Fatalf("GET users: %v", err)
	}
	defer response.Body.Close()
	var envelope struct {
		Data struct {
			Total int `json:"total"`
		} `json:"data"`
	}
	if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode users: %v", err)
	}
	if envelope.Data.Total != 4 {
		t.Fatalf("total = %d, want 4", envelope.Data.Total)
	}
	if got := upstream.Count("users-page-1"); got != 1 {
		t.Fatalf("users-page-1 count = %d, want 1", got)
	}
	if got := len(upstream.Requests()); got != 2 {
		t.Fatalf("request count = %d, want 2", got)
	}
}

func TestAllFakeUpstreamScenariosLoad(t *testing.T) {
	manifest := loadUpstreamManifest(t, DesignFixturePath("f02-upstream"))
	supported := manifest.Scenarios["supported"]
	if !routeExists(supported.Routes, "performance-summary-24h") {
		t.Fatal("F02 supported scenario is missing performance summary route")
	}
	if !routeExists(manifest.Scenarios["performance_invalid"].Routes, "performance-summary-invalid") {
		t.Fatal("F02 performance_invalid scenario is missing invalid performance route")
	}
	for scenario := range manifest.Scenarios {
		t.Run(scenario, func(t *testing.T) {
			upstream := NewFakeUpstream(t, scenario)
			if upstream.URL() == "" {
				t.Fatal("empty upstream URL")
			}
		})
	}
}

func routeExists(routes []upstreamRoute, id string) bool {
	for _, route := range routes {
		if route.ID == id {
			return true
		}
	}
	return false
}
