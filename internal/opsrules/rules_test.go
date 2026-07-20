package opsrules

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

type ruleFile struct {
	Groups []ruleGroup `yaml:"groups"`
}

type ruleGroup struct {
	Name  string     `yaml:"name"`
	Rules []ruleItem `yaml:"rules"`
}

type ruleItem struct {
	Alert  string            `yaml:"alert"`
	Record string            `yaml:"record"`
	Expr   string            `yaml:"expr"`
	For    string            `yaml:"for"`
	Labels map[string]string `yaml:"labels"`
}

type prometheusConfig struct {
	RuleFiles     []string       `yaml:"rule_files"`
	ScrapeConfigs []scrapeConfig `yaml:"scrape_configs"`
}

type scrapeConfig struct {
	JobName     string `yaml:"job_name"`
	MetricsPath string `yaml:"metrics_path"`
}

func TestPrometheusRulesAreCompleteAndLowCardinality(t *testing.T) {
	root := filepath.Join("..", "..")
	recording := readRuleFile(t, filepath.Join(root, "deploy", "prometheus", "recording-rules.yaml"))
	alerts := readRuleFile(t, filepath.Join(root, "deploy", "prometheus", "alert-rules.yaml"))
	requiredAlerts := map[string]bool{
		"NewAPIPilotTargetDown": false, "NewAPIPilotScrapeMissing": false,
		"NewAPIPilotSchedulerMetricMissing": false, "NewAPIPilotSchedulerStalled": false,
		"NewAPIPilotNotReady": false, "NewAPIPilotRuntimeComponentNotReady": false,
		"NewAPIPilotHTTPErrorRateHigh": false, "NewAPIPilotUpstreamFailureRateHigh": false,
		"NewAPIPilotTaskQueueStalled": false, "NewAPIPilotCollectionLagging": false,
		"NewAPIPilotCollectionSitesStale": false, "NewAPIPilotCollectionFailures": false,
		"NewAPIPilotDingTalkDeliveryFailures": false,
		"NewAPIPilotExportFailures":           false, "NewAPIPilotDBPoolPressure": false,
		"NewAPIPilotDBPoolMetricInvalid": false, "NewAPIPilotExportFilesystemInvalid": false,
		"NewAPIPilotExportFilesystemLow": false, "NewAPIPilotBackupStale": false,
		"NewAPIPilotBackupFailure": false, "NewAPIPilotBackupMetricMissing": false,
		"NewAPIPilotClockMetricMissing": false, "NewAPIPilotClockOffsetHigh": false,
	}
	records := map[string]bool{
		"new_api_pilot:http_5xx_ratio:5m":           false,
		"new_api_pilot:upstream_failure_ratio:5m":   false,
		"new_api_pilot:db_pool_utilization":         false,
		"new_api_pilot:alert_delivery_failures:10m": false,
		"new_api_pilot:export_failures:15m":         false,
		"new_api_pilot:collection_failures:15m":     false,
	}
	seen := make(map[string]struct{})
	expressions := make(map[string]string)
	for _, file := range []ruleFile{recording, alerts} {
		for _, group := range file.Groups {
			if group.Name == "" || len(group.Rules) == 0 {
				t.Fatalf("invalid empty rule group: %+v", group)
			}
			for _, rule := range group.Rules {
				name := rule.Alert
				if name == "" {
					name = rule.Record
				}
				if name == "" || strings.TrimSpace(rule.Expr) == "" {
					t.Fatalf("rule name or expression is empty: %+v", rule)
				}
				if _, duplicate := seen[name]; duplicate {
					t.Fatalf("duplicate Prometheus rule %q", name)
				}
				seen[name] = struct{}{}
				expressions[name] = rule.Expr
				if _, required := requiredAlerts[name]; required {
					requiredAlerts[name] = true
					if rule.Labels["severity"] != "warning" && rule.Labels["severity"] != "critical" {
						t.Errorf("alert %s has invalid severity %q", name, rule.Labels["severity"])
					}
				}
				if _, required := records[name]; required {
					records[name] = true
				}
			}
		}
	}
	for name, fragment := range map[string]string{
		"new_api_pilot:collection_failures:15m": `event="completion",result=~"failed|exhausted|lost"`,
		"new_api_pilot:export_failures:15m":     `event="failure",result=~"failed|exhausted|lost"`,
	} {
		if !strings.Contains(strings.Join(strings.Fields(expressions[name]), ""), fragment) {
			t.Errorf("recording rule %s does not cover stable failure labels", name)
		}
	}
	for name, present := range requiredAlerts {
		if !present {
			t.Errorf("required alert %s is missing", name)
		}
	}
	for name, present := range records {
		if !present {
			t.Errorf("required recording rule %s is missing", name)
		}
	}

	for _, name := range []string{"recording-rules.yaml", "alert-rules.yaml"} {
		payload, err := os.ReadFile(filepath.Join(root, "deploy", "prometheus", name))
		if err != nil {
			t.Fatal(err)
		}
		lower := strings.ToLower(string(payload))
		for _, forbidden := range []string{
			"site_id", "user_id", "model_name", "channel_id", "request_id",
			"webhook", "access_token", "error_text",
		} {
			if strings.Contains(lower, forbidden) {
				t.Errorf("%s contains forbidden high-cardinality or secret label term %q", name, forbidden)
			}
		}
	}
}

func TestPrometheusExampleLoadsBothRuleFiles(t *testing.T) {
	path := filepath.Join("..", "..", "deploy", "prometheus", "prometheus.example.yaml")
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var configuration prometheusConfig
	if err := yaml.Unmarshal(payload, &configuration); err != nil {
		t.Fatalf("parse Prometheus example: %v", err)
	}
	joined := strings.Join(configuration.RuleFiles, "\n")
	for _, required := range []string{"recording-rules.yaml", "alert-rules.yaml"} {
		if !strings.Contains(joined, required) {
			t.Errorf("Prometheus example does not load %s", required)
		}
	}
	found := false
	infrastructureFound := false
	for _, scrape := range configuration.ScrapeConfigs {
		if scrape.JobName == "new-api-pilot" && scrape.MetricsPath == "/metrics" {
			found = true
		}
		if scrape.JobName == "new-api-pilot-infrastructure" && scrape.MetricsPath == "/metrics" {
			infrastructureFound = true
		}
	}
	if !found {
		t.Fatal("Prometheus example is missing the new-api-pilot /metrics scrape job")
	}
	if !infrastructureFound {
		t.Fatal("Prometheus example is missing the node-exporter textfile scrape job")
	}
}

func TestGrafanaDashboardReferencesOperationalMetrics(t *testing.T) {
	path := filepath.Join("..", "..", "deploy", "grafana", "new-api-pilot-dashboard.json")
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var dashboard struct {
		UID    string `json:"uid"`
		Panels []struct {
			Title   string `json:"title"`
			Targets []struct {
				Expr string `json:"expr"`
			} `json:"targets"`
		} `json:"panels"`
	}
	if err := json.Unmarshal(payload, &dashboard); err != nil {
		t.Fatalf("parse Grafana dashboard: %v", err)
	}
	if dashboard.UID != "new-api-pilot-ops" || len(dashboard.Panels) < 8 {
		t.Fatalf("Grafana dashboard is incomplete: uid=%q panels=%d", dashboard.UID, len(dashboard.Panels))
	}
	var expressions strings.Builder
	for _, panel := range dashboard.Panels {
		if strings.TrimSpace(panel.Title) == "" || len(panel.Targets) == 0 {
			t.Errorf("Grafana panel is incomplete: %+v", panel)
		}
		for _, target := range panel.Targets {
			expressions.WriteString(target.Expr)
			expressions.WriteByte('\n')
		}
	}
	for _, required := range []string{
		"new_api_pilot_ready", "new_api_pilot_scheduler_heartbeat_timestamp_seconds",
		"new_api_pilot_task_oldest_age_seconds", "new_api_pilot_collection_lag_seconds",
		"new_api_pilot:db_pool_utilization", "new_api_pilot_export_free_bytes",
		"new_api_pilot:alert_delivery_failures:10m", "new_api_pilot:export_failures:15m",
		"new_api_pilot_backup_last_success_timestamp_seconds", "new_api_pilot_clock_offset_seconds",
	} {
		if !strings.Contains(expressions.String(), required) {
			t.Errorf("Grafana dashboard does not reference %s", required)
		}
	}
}

func readRuleFile(t *testing.T, path string) ruleFile {
	t.Helper()
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var result ruleFile
	if err := yaml.Unmarshal(payload, &result); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	if len(result.Groups) == 0 {
		t.Fatalf("%s has no rule groups", path)
	}
	return result
}
