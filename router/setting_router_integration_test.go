package router

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"new-api-pilot/common"
	"new-api-pilot/config"
	"new-api-pilot/constant"
	"new-api-pilot/controller"
	"new-api-pilot/dto"
	"new-api-pilot/model"
	"new-api-pilot/service"
	testsupport "new-api-pilot/tests/support"
)

func TestSettingRoutesEnforceViewerReadAdminWriteWithoutSecretLeakage(t *testing.T) {
	tx := openSiteRouterTestTransaction(t)
	if err := testsupport.ResetPlatformSettings(context.Background(), tx, 1_752_400_800); err != nil {
		t.Fatalf("reset setting route fixtures: %v", err)
	}
	cipher, err := common.NewCipher([]byte("abcdefghijklmnopqrstuvwxyz123456"))
	if err != nil {
		t.Fatalf("create setting route cipher: %v", err)
	}
	settings, err := service.NewSettingService(service.SettingServiceOptions{
		Repository: model.NewSettingRepository(tx), Cipher: cipher,
		Clock:        testsupport.NewFakeClock(time.Unix(1_752_400_800, 0)),
		PublicOrigin: "https://pilot.example",
	})
	if err != nil {
		t.Fatalf("create setting route service: %v", err)
	}
	settingController := controller.NewSettingController(settings)
	viewer, err := New(Options{
		Config: config.Config{AppEnv: config.EnvironmentTest}, SettingController: settingController,
		IdentityResolver: fixedSiteIdentityResolver{role: constant.RoleViewer},
	})
	if err != nil {
		t.Fatalf("create viewer setting router: %v", err)
	}
	admin, err := New(Options{
		Config: config.Config{AppEnv: config.EnvironmentTest}, SettingController: settingController,
		IdentityResolver: fixedSiteIdentityResolver{role: constant.RoleAdmin},
	})
	if err != nil {
		t.Fatalf("create admin setting router: %v", err)
	}

	read := performSiteRequest(viewer, http.MethodGet, "/api/settings", "")
	readEnvelope := decodeSiteEnvelope(t, read)
	if read.Code != http.StatusOK || !readEnvelope.Success {
		t.Fatalf("viewer setting read = %d %#v body=%s", read.Code, readEnvelope, read.Body.String())
	}
	for _, forbidden := range []string{"setting_value", "v1:", "access_token="} {
		if strings.Contains(read.Body.String(), forbidden) {
			t.Fatalf("viewer setting response leaked %q: %s", forbidden, read.Body.String())
		}
	}
	var groups []dto.SettingGroup
	if err := json.Unmarshal(readEnvelope.Data, &groups); err != nil {
		t.Fatalf("decode setting groups: %v", err)
	}
	fullItems := make([]map[string]any, 0, 39)
	for _, group := range groups {
		for _, item := range group.Items {
			if item.ReadOnly {
				continue
			}
			patch := map[string]any{"key": item.Key}
			if !item.Secret {
				patch["value"] = item.Value
			}
			fullItems = append(fullItems, patch)
		}
	}
	if len(fullItems) != 39 {
		t.Fatalf("editable setting count = %d, want 39", len(fullItems))
	}
	fullBody, err := json.Marshal(map[string]any{"items": fullItems})
	if err != nil {
		t.Fatalf("marshal full setting patch: %v", err)
	}
	fullWrite := performSiteRequest(admin, http.MethodPut, "/api/settings", string(fullBody))
	fullEnvelope := decodeSiteEnvelope(t, fullWrite)
	if fullWrite.Code != http.StatusOK || !fullEnvelope.Success {
		t.Fatalf("39-item admin setting write = %d %#v body=%s", fullWrite.Code, fullEnvelope, fullWrite.Body.String())
	}

	viewerWrite := performSiteRequest(viewer, http.MethodPut, "/api/settings",
		`{"items":[{"key":"collector.usage_delay_minutes","value":4}]}`)
	viewerEnvelope := decodeSiteEnvelope(t, viewerWrite)
	if viewerWrite.Code != http.StatusForbidden || viewerEnvelope.Code != constant.CodeForbidden {
		t.Fatalf("viewer setting write = %d %#v", viewerWrite.Code, viewerEnvelope)
	}

	adminWrite := performSiteRequest(admin, http.MethodPut, "/api/settings",
		`{"items":[{"key":"collector.usage_delay_minutes","value":4}]}`)
	adminEnvelope := decodeSiteEnvelope(t, adminWrite)
	if adminWrite.Code != http.StatusOK || !adminEnvelope.Success {
		t.Fatalf("admin setting write = %d %#v body=%s", adminWrite.Code, adminEnvelope, adminWrite.Body.String())
	}
}
