package testsupport

import (
	"encoding/json"
	"math/big"
	"os"
	"testing"
	"unicode/utf8"

	"gopkg.in/yaml.v3"
)

func TestF01PasswordAndBigintBoundaries(t *testing.T) {
	content, err := os.ReadFile(DesignFixturePath("f01-auth.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		SchemaVersion int    `json:"schema_version"`
		FixtureID     string `json:"fixture_id"`
		Bigint        struct {
			MaximumSafe string `json:"javascript_max_safe_integer"`
			UserID      string `json:"platform_user_id"`
		} `json:"bigint_contract"`
		PasswordPolicy struct {
			MinimumCharacters int `json:"minimum_unicode_characters"`
			MaximumBytes      int `json:"maximum_utf8_bytes"`
			Valid             []struct {
				Value     string `json:"value"`
				UTF8Bytes int    `json:"utf8_bytes"`
			} `json:"valid"`
			Invalid []struct {
				Value     string `json:"value"`
				UTF8Bytes int    `json:"utf8_bytes"`
				Reason    string `json:"reason"`
			} `json:"invalid"`
		} `json:"password_policy"`
	}
	if err := json.Unmarshal(content, &fixture); err != nil {
		t.Fatal(err)
	}
	if fixture.SchemaVersion != 1 || fixture.FixtureID != "F01" {
		t.Fatalf("unexpected F01 identity: version=%d id=%q", fixture.SchemaVersion, fixture.FixtureID)
	}
	maximumSafe, ok := new(big.Int).SetString(fixture.Bigint.MaximumSafe, 10)
	if !ok {
		t.Fatal("invalid maximum safe integer")
	}
	userID, ok := new(big.Int).SetString(fixture.Bigint.UserID, 10)
	if !ok || userID.Cmp(maximumSafe) <= 0 {
		t.Fatalf("fixture user ID %q is not above JavaScript's safe integer", fixture.Bigint.UserID)
	}
	for _, candidate := range fixture.PasswordPolicy.Valid {
		if got := len([]byte(candidate.Value)); got != candidate.UTF8Bytes {
			t.Fatalf("valid password byte length = %d, fixture says %d", got, candidate.UTF8Bytes)
		}
		if utf8.RuneCountInString(candidate.Value) < fixture.PasswordPolicy.MinimumCharacters || candidate.UTF8Bytes > fixture.PasswordPolicy.MaximumBytes {
			t.Fatalf("valid password does not satisfy fixture policy")
		}
	}
	for _, candidate := range fixture.PasswordPolicy.Invalid {
		if candidate.UTF8Bytes > 0 && len([]byte(candidate.Value)) != candidate.UTF8Bytes {
			t.Fatalf("invalid password byte length = %d, fixture says %d", len([]byte(candidate.Value)), candidate.UTF8Bytes)
		}
		if candidate.Reason == "too_short" && utf8.RuneCountInString(candidate.Value) >= fixture.PasswordPolicy.MinimumCharacters {
			t.Fatalf("too_short fixture has enough characters")
		}
		if candidate.Reason == "too_many_utf8_bytes" && len([]byte(candidate.Value)) <= fixture.PasswordPolicy.MaximumBytes {
			t.Fatalf("too_many_utf8_bytes fixture is not over the limit")
		}
	}
}

func TestF04AndF05FixtureIdentity(t *testing.T) {
	f04Content, err := os.ReadFile(DesignFixturePath("f04-state-machines.json"))
	if err != nil {
		t.Fatal(err)
	}
	var f04 struct {
		SchemaVersion int    `json:"schema_version"`
		FixtureID     string `json:"fixture_id"`
		Scenarios     []struct {
			ID string `json:"id"`
		} `json:"scenarios"`
	}
	if err := json.Unmarshal(f04Content, &f04); err != nil {
		t.Fatal(err)
	}
	scenarioIDs := make(map[string]struct{}, len(f04.Scenarios))
	for _, scenario := range f04.Scenarios {
		scenarioIDs[scenario.ID] = struct{}{}
	}
	_, hasFastTaskRedis := scenarioIDs["fast_task_redis_history"]
	_, hasPerformanceCache := scenarioIDs["performance_proxy_cache_fence"]
	if f04.SchemaVersion != 1 || f04.FixtureID != "F04" || !hasFastTaskRedis || !hasPerformanceCache {
		t.Fatalf("unexpected F04 identity or scenario count")
	}

	f05Content, err := os.ReadFile(DesignFixturePath("f05-ops-capacity.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var f05 struct {
		SchemaVersion int    `yaml:"schema_version"`
		FixtureID     string `yaml:"fixture_id"`
		Capacity      struct {
			Sites int `yaml:"sites"`
		} `yaml:"capacity"`
		Redis struct {
			RequiredInProduction  bool `yaml:"required_in_production"`
			UnavailableHTTPStatus int  `yaml:"unavailable_http_status"`
		} `yaml:"redis"`
		PerformanceProxyCache struct {
			CachedHours int  `yaml:"cached_hours"`
			Persistent  bool `yaml:"persistent"`
		} `yaml:"performance_proxy_cache"`
	}
	if err := yaml.Unmarshal(f05Content, &f05); err != nil {
		t.Fatal(err)
	}
	if f05.SchemaVersion != 2 || f05.FixtureID != "F05" || f05.Capacity.Sites != 50 ||
		!f05.Redis.RequiredInProduction || f05.Redis.UnavailableHTTPStatus != 503 ||
		f05.PerformanceProxyCache.CachedHours != 24 || f05.PerformanceProxyCache.Persistent {
		t.Fatalf("unexpected F05 identity or capacity")
	}
}
