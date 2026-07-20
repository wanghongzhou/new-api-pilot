package integration_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"gorm.io/gorm"

	"new-api-pilot/common"
	"new-api-pilot/constant"
	"new-api-pilot/dto"
	"new-api-pilot/model"
	"new-api-pilot/service"
)

const coreAcceptanceDatabaseLock = "new-api-pilot-core-acceptance"

var coreAcceptanceSequence atomic.Int64

func openCoreAcceptanceTransaction(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv("TEST_DATABASE_DSN"))
	if dsn == "" {
		if coreAcceptanceIDRequiresDatabase(strings.TrimSpace(os.Getenv("ACCEPTANCE_ID"))) {
			t.Fatalf("%s requires TEST_DATABASE_DSN", os.Getenv("ACCEPTANCE_ID"))
		}
		t.Skip("TEST_DATABASE_DSN is not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	database, err := model.Open(ctx, model.Options{DSN: dsn, MaxIdle: 2, MaxOpen: 10, MaxLifetime: time.Minute})
	if err != nil {
		t.Fatalf("open core acceptance database: %v", err)
	}
	connection, err := database.SQL.Conn(ctx)
	if err != nil {
		_ = database.Close()
		t.Fatalf("reserve core acceptance lock connection: %v", err)
	}
	var acquired sql.NullInt64
	if err := connection.QueryRowContext(ctx, "SELECT GET_LOCK(?, 60)", coreAcceptanceDatabaseLock).Scan(&acquired); err != nil || !acquired.Valid || acquired.Int64 != 1 {
		_ = connection.Close()
		_ = database.Close()
		t.Fatalf("acquire core acceptance lock = %v, %v", acquired, err)
	}
	if err := model.NewMigrationRunner(database.SQL).Run(ctx); err != nil {
		_, _ = connection.ExecContext(context.Background(), "SELECT RELEASE_LOCK(?)", coreAcceptanceDatabaseLock)
		_ = connection.Close()
		_ = database.Close()
		t.Fatalf("run core acceptance migrations: %v", err)
	}
	tx := database.GORM.Begin()
	if tx.Error != nil {
		_, _ = connection.ExecContext(context.Background(), "SELECT RELEASE_LOCK(?)", coreAcceptanceDatabaseLock)
		_ = connection.Close()
		_ = database.Close()
		t.Fatalf("begin core acceptance transaction: %v", tx.Error)
	}
	t.Cleanup(func() {
		_ = tx.Rollback().Error
		cleanup, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		_, _ = connection.ExecContext(cleanup, "SELECT RELEASE_LOCK(?)", coreAcceptanceDatabaseLock)
		_ = connection.Close()
		_ = database.Close()
	})
	return tx
}

func coreAcceptanceIDRequiresDatabase(id string) bool {
	switch id {
	case "A01", "A02", "A03", "A04", "A06", "A07", "A10", "A11", "A19", "A21", "A24", "A32", "A33", "A34", "A35", "A36", "A37", "A41", "A54", "A56", "A57", "A63", "A80", "A81", "A86", "A102":
		return true
	default:
		return false
	}
}

func coreAcceptanceName(prefix string) string {
	return fmt.Sprintf("%s-%d", prefix, coreAcceptanceSequence.Add(1))
}

func newCoreCipher(t *testing.T) *common.Cipher {
	t.Helper()
	cipher, err := common.NewCipher([]byte("core-acceptance-0123456789abcdef"))
	if err != nil {
		t.Fatalf("create core acceptance cipher: %v", err)
	}
	return cipher
}

func createCorePendingSite(t *testing.T, database *gorm.DB, now int64) model.Site {
	t.Helper()
	name := coreAcceptanceName("site")
	site := model.Site{
		Name: name, BaseURL: "https://" + name + ".example.test", ConfigVersion: 1,
		ManagementStatus: constant.SiteManagementActive, OnlineStatus: constant.SiteOnlineUnknown,
		AuthStatus: constant.SiteAuthUnauthorized, StatisticsStatus: constant.SiteStatisticsPendingConfig,
		HealthStatus: constant.SiteHealthUnavailable, CreatedAt: now, UpdatedAt: now,
	}
	if err := model.NewSiteRepository(database).Create(context.Background(), &site); err != nil {
		t.Fatalf("create core pending site: %v", err)
	}
	return site
}

func createCoreAuthorizedSite(t *testing.T, database *gorm.DB, cipher *common.Cipher, now int64) model.Site {
	t.Helper()
	site := createCorePendingSite(t, database, now)
	rootID := int64(1)
	rootCreatedAt := now - 7200
	statisticsStartAt := coreFloorHour(rootCreatedAt)
	statisticsStartSource := "root_created_at"
	token, err := cipher.Encrypt([]byte("core-existing-token"), "site:"+strconv.FormatInt(site.ID, 10)+":access_token")
	if err != nil {
		t.Fatalf("encrypt core site token: %v", err)
	}
	rate := "1"
	if err := database.Model(&model.Site{}).Where("id = ?", site.ID).Updates(map[string]any{
		"root_user_id": rootID, "root_created_at": rootCreatedAt, "statistics_start_at": statisticsStartAt,
		"statistics_start_source": statisticsStartSource, "access_token_encrypted": token,
		"auth_status": constant.SiteAuthAuthorized, "statistics_status": constant.SiteStatisticsReady,
		"data_export_enabled": true, "quota_per_unit": rate, "usd_exchange_rate": rate,
		"last_rate_at": now, "updated_at": now,
	}).Error; err != nil {
		t.Fatalf("authorize core site: %v", err)
	}
	if err := storeCorePassingCapabilities(database, site.ID, now); err != nil {
		t.Fatalf("store core site capabilities: %v", err)
	}
	persisted, err := model.NewSiteRepository(database).FindByID(context.Background(), site.ID)
	if err != nil {
		t.Fatalf("reload core authorized site: %v", err)
	}
	return persisted
}

func storeCorePassingCapabilities(database *gorm.DB, siteID, checkedAt int64) error {
	capabilities := make([]model.SiteCapability, 0, len(constant.SiteCapabilityKeys()))
	for _, key := range constant.SiteCapabilityKeys() {
		params, err := json.Marshal(map[string]any{
			"site_id": strconv.FormatInt(siteID, 10), "capability_key": key,
		})
		if err != nil {
			return err
		}
		status := constant.CapabilityStatusPassed
		if key == constant.CapabilityFlowDataConsistency {
			status = constant.CapabilityStatusSkipped
		}
		capabilities = append(capabilities, model.SiteCapability{
			SiteID: siteID, CapabilityKey: key, Status: status,
			MessageCode: string(constant.MessageCapabilityOK), MessageParams: params, CheckedAt: checkedAt,
		})
	}
	return model.NewSiteRepository(database).ReplaceCapabilities(context.Background(), siteID, capabilities)
}

func createCoreCustomer(t *testing.T, database *gorm.DB, now int64, status string) model.Customer {
	t.Helper()
	customer := model.Customer{
		Name: coreAcceptanceName("customer"), Status: status, StatisticsBackfillStatus: "none",
		CreatedAt: now, UpdatedAt: now,
	}
	if err := model.NewCustomerRepository(database).Create(context.Background(), &customer); err != nil {
		t.Fatalf("create core customer: %v", err)
	}
	return customer
}

func createCoreAccount(t *testing.T, database *gorm.DB, siteID, customerID, remoteUserID, now int64) model.Account {
	t.Helper()
	observedAt := now - 60
	account := model.Account{
		SiteID: siteID, CustomerID: customerID, RemoteUserID: remoteUserID, RemoteCreatedAt: now - 7200,
		Username: "remote-" + strconv.FormatInt(remoteUserID, 10), RemoteStatus: 1,
		RemoteState: model.AccountRemoteStateNormal, ManagedStatus: model.AccountManagedStatusActive,
		StatisticsBackfillStatus: "none", LastRemoteSeenAt: &observedAt, LastSyncedAt: &observedAt,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := model.NewAccountRepository(database).Create(context.Background(), &account); err != nil {
		t.Fatalf("create core account: %v", err)
	}
	return account
}

func coreFloorHour(value int64) int64 {
	return value - value%3600
}

type coreSiteClient struct {
	self             dto.UpstreamIdentity
	root             dto.UpstreamUser
	users            map[int64]dto.UpstreamUser
	snapshot         dto.UpstreamUserSnapshot
	channels         dto.UpstreamChannelSnapshot
	status           dto.UpstreamStatus
	logStat          dto.UpstreamLogStat
	selfErr          error
	getUserErr       error
	snapshotErr      error
	channelsErr      error
	statusErr        error
	instancesErr     error
	logStatErr       error
	flowErr          error
	dataErr          error
	instances        []dto.UpstreamInstance
	publicLoginToken string
}

func newCoreSiteClient(now int64) *coreSiteClient {
	root := dto.UpstreamUser{
		UpstreamIdentity: dto.UpstreamIdentity{ID: 1, Username: "root", DisplayName: "Root", Role: 100, Status: 1},
		CreatedAt:        now - 7200,
	}
	return &coreSiteClient{
		self: root.UpstreamIdentity, root: root, users: map[int64]dto.UpstreamUser{root.ID: root},
		snapshot: dto.UpstreamUserSnapshot{Total: 1, Items: []dto.UpstreamUser{root}},
		channels: dto.UpstreamChannelSnapshot{Total: 1, Items: []dto.UpstreamChannel{{ID: 1, Name: "core-channel", Status: 1}}},
		status:   dto.UpstreamStatus{Version: "core-v1", SystemName: "Core", QuotaPerUnit: "1", USDExchangeRate: "1", DataExportEnabled: true},
		logStat:  dto.UpstreamLogStat{RPM: 1, TPM: 1, Quota: 1}, publicLoginToken: "core-rotated-token",
	}
}

func (client *coreSiteClient) Status(context.Context, string) (dto.UpstreamStatus, error) {
	return client.status, client.statusErr
}

func (client *coreSiteClient) Self(context.Context, string) (dto.UpstreamIdentity, error) {
	return client.self, client.selfErr
}

func (client *coreSiteClient) GetUser(_ context.Context, _ string, id int64) (dto.UpstreamUser, error) {
	if client.getUserErr != nil {
		return dto.UpstreamUser{}, client.getUserErr
	}
	if user, exists := client.users[id]; exists {
		return user, nil
	}
	return dto.UpstreamUser{}, service.ErrUpstreamUserNotFound
}

func (client *coreSiteClient) SnapshotUsers(context.Context, string) (dto.UpstreamUserSnapshot, error) {
	return client.snapshot, client.snapshotErr
}

func (client *coreSiteClient) SnapshotChannels(context.Context, string) (dto.UpstreamChannelSnapshot, error) {
	return client.channels, client.channelsErr
}

func (client *coreSiteClient) FlowHour(context.Context, string, int64) ([]dto.UpstreamFlowRow, error) {
	return nil, client.flowErr
}

func (client *coreSiteClient) DataHour(context.Context, string, int64) ([]dto.UpstreamDataRow, error) {
	return nil, client.dataErr
}

func (client *coreSiteClient) Instances(context.Context, string) ([]dto.UpstreamInstance, error) {
	return client.instances, client.instancesErr
}

func (client *coreSiteClient) LogStat(context.Context, string) (dto.UpstreamLogStat, error) {
	return client.logStat, client.logStatErr
}

func (client *coreSiteClient) PerformanceSummary(context.Context, string, int) (dto.UpstreamPerformanceSummary, error) {
	return dto.UpstreamPerformanceSummary{}, nil
}

func (client *coreSiteClient) LoginAndGenerateAccessToken(context.Context, string, string, string) (dto.UpstreamIdentity, string, error) {
	return client.self, client.publicLoginToken, nil
}

func (client *coreSiteClient) CloseIdleConnections() {}

func (client *coreSiteClient) ListUsersPage(context.Context, string, int) (dto.UpstreamUserPage, error) {
	return dto.UpstreamUserPage{Page: 1, PageSize: len(client.snapshot.Items), Total: client.snapshot.Total, Items: client.snapshot.Items}, client.snapshotErr
}

func (client *coreSiteClient) SearchUsers(context.Context, string, string, int) (dto.UpstreamUserPage, error) {
	return dto.UpstreamUserPage{Page: 1, PageSize: len(client.snapshot.Items), Total: client.snapshot.Total, Items: client.snapshot.Items}, client.snapshotErr
}

type coreSiteClientFactory struct {
	client         *coreSiteClient
	publicCalls    int
	authenticated  int
	authTokens     []string
	authOrigins    []string
	authRootUserID []int64
}

func (factory *coreSiteClientFactory) NewPublic(string) (service.SiteUpstreamClient, error) {
	factory.publicCalls++
	return factory.client, nil
}

func (factory *coreSiteClientFactory) NewAuthenticated(_ string, origin, token string, rootUserID int64) (service.SiteUpstreamClient, error) {
	factory.authenticated++
	factory.authTokens = append(factory.authTokens, token)
	factory.authOrigins = append(factory.authOrigins, origin)
	factory.authRootUserID = append(factory.authRootUserID, rootUserID)
	return factory.client, nil
}
