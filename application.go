package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync/atomic"

	"new-api-pilot/common"
	"new-api-pilot/config"
	"new-api-pilot/constant"
	"new-api-pilot/controller"
	"new-api-pilot/middleware"
	"new-api-pilot/model"
	"new-api-pilot/router"
	"new-api-pilot/service"
	"new-api-pilot/webui"
	"new-api-pilot/worker"
)

type runtimeLifecycle interface {
	Start(context.Context) error
	Quiesce() error
	Stop(context.Context) error
	Ready() bool
}

type application struct {
	Handler   http.Handler
	Readiness *common.Readiness
	Metrics   *common.Metrics
	runtime   runtimeLifecycle
}

type applicationOptions struct {
	Config       config.Config
	Database     *model.Database
	Cipher       *common.Cipher
	Clock        common.Clock
	RuntimeMode  applicationRuntimeMode
	AcceptanceID string
}

type applicationRuntimeMode uint8

const (
	applicationRuntimeStandard applicationRuntimeMode = iota
	applicationRuntimeA49ReadOnly
)

type acceptanceReadOnlyRuntime struct {
	ready atomic.Bool
}

func (runtime *acceptanceReadOnlyRuntime) Start(context.Context) error {
	runtime.ready.Store(true)
	return nil
}

func (runtime *acceptanceReadOnlyRuntime) Quiesce() error {
	runtime.ready.Store(false)
	return nil
}

func (runtime *acceptanceReadOnlyRuntime) Stop(context.Context) error {
	runtime.ready.Store(false)
	return nil
}

func (runtime *acceptanceReadOnlyRuntime) Ready() bool { return runtime.ready.Load() }

func bootstrapApplication(
	ctx context.Context,
	options applicationOptions,
) (*application, service.BootstrapResult, error) {
	if options.Database == nil || options.Database.GORM == nil || options.Database.SQL == nil ||
		options.Cipher == nil || options.Clock == nil {
		return nil, service.BootstrapResult{}, errors.New("application dependencies are required")
	}
	if err := validateApplicationRuntimeOptions(options); err != nil {
		return nil, service.BootstrapResult{}, err
	}
	metrics := common.NewMetrics()
	redisStore, err := common.NewRedisStore(options.Config.RedisDSN, options.Config.RedisDB, options.Config.RedisTimeout, options.Config.FastTaskRetention, options.Config.FastTaskHistoryCount)
	if err != nil {
		return nil, service.BootstrapResult{}, fmt.Errorf("initialize redis: %w", err)
	}

	userRepository := model.NewPlatformUserRepository(options.Database.GORM)
	userService := service.NewPlatformUserService(userRepository, options.Clock)
	bootstrap, err := userService.EnsureBootstrapAdmin(ctx, options.Config.BootstrapAdminSecret)
	if err != nil {
		return nil, service.BootstrapResult{}, fmt.Errorf("initialize bootstrap admin: %w", err)
	}
	sessionStore, err := common.NewSessionStore(
		options.Config.SessionSecret,
		options.Config.SessionCookieSecure,
		options.Clock,
	)
	if err != nil {
		return nil, service.BootstrapResult{}, fmt.Errorf("initialize session store: %w", err)
	}
	authService, err := service.NewAuthService(userRepository, service.NewLoginLimiter(options.Clock), options.Clock)
	if err != nil {
		return nil, service.BootstrapResult{}, fmt.Errorf("initialize auth service: %w", err)
	}
	settingService, err := service.NewSettingService(service.SettingServiceOptions{
		Repository:    model.NewSettingRepository(options.Database.GORM),
		Cipher:        options.Cipher,
		Clock:         options.Clock,
		AppEnv:        options.Config.AppEnv,
		PublicOrigin:  options.Config.PublicOrigin,
		DingTalkHosts: options.Config.DingTalkAllowedHosts,
	})
	if err != nil {
		return nil, service.BootstrapResult{}, fmt.Errorf("initialize setting service: %w", err)
	}
	alertService, err := service.NewAlertService(service.AlertServiceOptions{
		Database: options.Database.GORM, Clock: options.Clock, Metrics: metrics,
	})
	if err != nil {
		return nil, service.BootstrapResult{}, fmt.Errorf("initialize alert service: %w", err)
	}
	alertHook, err := service.NewAlertPostCommitHook(alertService)
	if err != nil {
		return nil, service.BootstrapResult{}, fmt.Errorf("initialize alert post-commit hook: %w", err)
	}
	postCommit, err := service.NewAlertPostCommitCoordinator(service.AlertPostCommitCoordinatorOptions{
		Database: options.Database.GORM, Hook: alertHook, Clock: options.Clock,
	})
	if err != nil {
		return nil, service.BootstrapResult{}, fmt.Errorf("initialize alert post-commit coordinator: %w", err)
	}
	dingTalkService, err := service.NewDingTalkService(service.DingTalkServiceOptions{
		Database: options.Database.GORM, Clock: options.Clock, Cipher: options.Cipher,
		AllowedHosts: options.Config.DingTalkAllowedHosts, PublicOrigin: options.Config.PublicOrigin,
		Metrics: metrics,
	})
	if err != nil {
		return nil, service.BootstrapResult{}, fmt.Errorf("initialize dingtalk service: %w", err)
	}
	clientFactory := service.NewConfiguredSiteClientFactory(service.SiteClientFactoryOptions{
		AllowedHostSuffixes: options.Config.UpstreamAllowedHostSuffixes,
		AllowedCIDRs:        options.Config.UpstreamAllowedCIDRs,
		CAFile:              options.Config.UpstreamCAFile,
		ConnectTimeout:      options.Config.UpstreamConnectTimeout,
		HeaderTimeout:       options.Config.UpstreamHeaderTimeout,
		RequestTimeout:      options.Config.UpstreamRequestTimeout,
		ExportTimeout:       options.Config.UpstreamExportTimeout,
		Metrics:             metrics,
	})
	maintenanceWake := worker.NewDataMaintenanceWake()
	siteRepository := model.NewSiteRepository(options.Database.GORM)
	siteService, err := service.NewSiteService(service.SiteServiceOptions{
		Repository: siteRepository, ClientFactory: clientFactory, Cipher: options.Cipher,
		Clock:           options.Clock,
		PreflightSecret: options.Config.SessionSecret, PostCommit: postCommit,
		Maintenance: maintenanceWake,
	})
	if err != nil {
		return nil, service.BootstrapResult{}, fmt.Errorf("initialize site service: %w", err)
	}
	statisticsService, err := service.NewStatisticsService(service.StatisticsServiceOptions{
		Database: options.Database.GORM, Clock: options.Clock,
	})
	if err != nil {
		return nil, service.BootstrapResult{}, fmt.Errorf("initialize statistics service: %w", err)
	}
	logService, err := service.NewUpstreamLogService(service.UpstreamLogServiceOptions{
		Database: options.Database.GORM, SiteRepository: siteRepository, ClientFactory: clientFactory,
		Cipher: options.Cipher, Clock: options.Clock,
	})
	if err != nil {
		return nil, service.BootstrapResult{}, fmt.Errorf("initialize upstream log service: %w", err)
	}
	inventoryService, err := service.NewUserInventoryService(options.Database.GORM)
	if err != nil {
		return nil, service.BootstrapResult{}, fmt.Errorf("initialize user inventory service: %w", err)
	}
	channelInventoryService, err := service.NewChannelInventoryService(options.Database.GORM)
	if err != nil {
		return nil, service.BootstrapResult{}, fmt.Errorf("initialize channel inventory service: %w", err)
	}
	performanceHistoryService, err := service.NewPerformanceHistoryService(options.Database.GORM)
	if err != nil {
		return nil, service.BootstrapResult{}, fmt.Errorf("initialize performance history service: %w", err)
	}
	financeOperationsService, err := service.NewFinanceOperationsService(options.Database.GORM, options.Clock)
	if err != nil {
		return nil, service.BootstrapResult{}, fmt.Errorf("initialize finance operations service: %w", err)
	}
	upstreamTaskService, err := service.NewUpstreamTaskService(options.Database.GORM)
	if err != nil {
		return nil, service.BootstrapResult{}, fmt.Errorf("initialize upstream task service: %w", err)
	}
	modelCatalogService, err := service.NewModelCatalogService(options.Database.GORM)
	if err != nil {
		return nil, service.BootstrapResult{}, fmt.Errorf("initialize model catalog service: %w", err)
	}
	localRankingService, err := service.NewLocalRankingService(options.Database.GORM, options.Clock)
	if err != nil {
		return nil, service.BootstrapResult{}, fmt.Errorf("initialize local ranking service: %w", err)
	}
	subscriptionPlanService, err := service.NewSubscriptionPlanService(options.Database.GORM)
	if err != nil {
		return nil, service.BootstrapResult{}, fmt.Errorf("initialize subscription plan service: %w", err)
	}
	pricingCatalogService, err := service.NewPricingCatalogService(options.Database.GORM)
	if err != nil {
		return nil, service.BootstrapResult{}, fmt.Errorf("initialize pricing catalog service: %w", err)
	}
	systemTaskCatalogService, err := service.NewSystemTaskCatalogService(options.Database.GORM)
	if err != nil {
		return nil, service.BootstrapResult{}, fmt.Errorf("initialize system task catalog service: %w", err)
	}
	exportService, err := service.NewExportService(service.ExportServiceOptions{
		Database: options.Database.GORM, Clock: options.Clock, ExportDir: options.Config.ExportDir,
	})
	if err != nil {
		return nil, service.BootstrapResult{}, fmt.Errorf("initialize export service: %w", err)
	}
	customerService, err := service.NewCustomerService(service.CustomerServiceOptions{
		Database: options.Database.GORM, Clock: options.Clock, Statistics: statisticsService, PostCommit: postCommit,
	})
	if err != nil {
		return nil, service.BootstrapResult{}, fmt.Errorf("initialize customer service: %w", err)
	}
	accountService, err := service.NewAccountService(service.AccountServiceOptions{
		Database: options.Database.GORM, ClientFactory: clientFactory, Cipher: options.Cipher,
		Clock: options.Clock, Statistics: statisticsService, PostCommit: postCommit,
	})
	if err != nil {
		return nil, service.BootstrapResult{}, fmt.Errorf("initialize account service: %w", err)
	}
	dashboardReader, err := service.NewDashboardReader(service.DashboardReaderOptions{
		Database: options.Database.GORM, Alerts: alertService, Clock: options.Clock,
	})
	if err != nil {
		return nil, service.BootstrapResult{}, fmt.Errorf("initialize dashboard readers: %w", err)
	}
	dashboardService, err := service.NewDashboardService(service.DashboardServiceOptions{
		Statistics: statisticsService, SiteHealth: dashboardReader, Alerts: dashboardReader,
		Realtime: dashboardReader, Clock: options.Clock,
	})
	if err != nil {
		return nil, service.BootstrapResult{}, fmt.Errorf("initialize dashboard service: %w", err)
	}
	applicationRuntime, err := buildApplicationRuntime(options, metrics, alertService, dingTalkService,
		clientFactory, siteRepository, siteService, logService, postCommit, maintenanceWake, redisStore)
	if err != nil {
		return nil, service.BootstrapResult{}, err
	}

	readiness := common.NewReadiness()
	readiness.AddCheck("database", options.Database.SQL.PingContext)
	readiness.AddCheck("redis", redisStore.Ping)
	identityResolver := middleware.SessionIdentityResolver{Store: sessionStore, Loader: authService}
	engine, err := router.New(router.Options{
		Config: options.Config, Database: options.Database.SQL, Readiness: readiness, Metrics: metrics,
		AuthController:               controller.NewAuthController(authService, userService, sessionStore),
		UserController:               controller.NewPlatformUserController(userService),
		SiteController:               controller.NewSiteController(siteService),
		CustomerController:           controller.NewCustomerController(customerService, accountService),
		AccountController:            controller.NewAccountController(accountService),
		StatisticsController:         controller.NewStatisticsController(statisticsService),
		ExportController:             controller.NewExportController(exportService),
		DashboardController:          controller.NewDashboardController(dashboardService),
		AlertController:              controller.NewAlertController(alertService),
		SettingController:            controller.NewSettingController(settingService),
		NotificationController:       controller.NewNotificationController(dingTalkService),
		FastTaskController:           controller.NewFastTaskController(redisStore),
		LogController:                controller.NewLogController(logService),
		UserInventoryController:      controller.NewUserInventoryController(inventoryService),
		ChannelInventoryController:   controller.NewChannelInventoryController(channelInventoryService),
		PerformanceHistoryController: controller.NewPerformanceHistoryController(performanceHistoryService),
		FinanceOperationsController:  controller.NewFinanceOperationsController(financeOperationsService),
		UpstreamTaskController:       controller.NewUpstreamTaskController(upstreamTaskService),
		ModelCatalogController:       controller.NewModelCatalogController(modelCatalogService),
		LocalRankingController:       controller.NewLocalRankingController(localRankingService),
		SubscriptionPlanController:   controller.NewSubscriptionPlanController(subscriptionPlanService),
		PricingCatalogController:     controller.NewPricingCatalogController(pricingCatalogService),
		SystemTaskCatalogController:  controller.NewSystemTaskCatalogController(systemTaskCatalogService),
		IdentityResolver:             identityResolver,
		WebAssets:                    webui.Assets(),
	})
	if err != nil {
		return nil, service.BootstrapResult{}, fmt.Errorf("create HTTP router: %w", err)
	}
	return &application{Handler: engine, Readiness: readiness, Metrics: metrics, runtime: applicationRuntime}, bootstrap, nil
}

func validateApplicationRuntimeOptions(options applicationOptions) error {
	switch options.RuntimeMode {
	case applicationRuntimeStandard:
		if options.AcceptanceID != "" {
			return errors.New("standard runtime cannot carry an acceptance identifier")
		}
	case applicationRuntimeA49ReadOnly:
		if options.Config.AppEnv != config.EnvironmentTest || options.AcceptanceID != a49AcceptanceID {
			return errors.New("A49 read-only runtime requires APP_ENV=test and ACCEPTANCE_ID=A49")
		}
	default:
		return errors.New("application runtime mode is invalid")
	}
	return nil
}

func buildApplicationRuntime(
	options applicationOptions,
	metrics *common.Metrics,
	alertService *service.AlertService,
	dingTalkService *service.DingTalkService,
	clientFactory service.SiteClientFactory,
	siteRepository *model.SiteRepository,
	siteService *service.SiteService,
	logService *service.UpstreamLogService,
	postCommit service.PostCommitNotifier,
	runtimeExtras ...any,
) (runtimeLifecycle, error) {
	var redisStore *common.RedisStore
	var maintenanceWake *worker.DataMaintenanceWake
	for _, extra := range runtimeExtras {
		switch typed := extra.(type) {
		case *common.RedisStore:
			redisStore = typed
		case *worker.DataMaintenanceWake:
			maintenanceWake = typed
		}
	}
	if maintenanceWake == nil {
		maintenanceWake = worker.NewDataMaintenanceWake()
	}
	if options.RuntimeMode == applicationRuntimeA49ReadOnly {
		return &acceptanceReadOnlyRuntime{}, nil
	}
	alertScanner, err := service.NewAlertEvaluationScanner(service.AlertEvaluationScannerOptions{
		Database: options.Database.GORM, Evaluator: alertService, Clock: options.Clock,
	})
	if err != nil {
		return nil, fmt.Errorf("initialize alert evaluation scanner: %w", err)
	}
	alertRuntime, err := worker.NewAlertRuntime(worker.AlertRuntimeOptions{
		Evaluator: alertScanner, Deliveries: dingTalkService, Clock: options.Clock,
		Metrics: metrics,
	})
	if err != nil {
		return nil, fmt.Errorf("initialize alert runtime: %w", err)
	}
	usageCollectionService, err := service.NewUsageCollectionService(service.UsageCollectionServiceOptions{
		Repository: siteRepository, ClientFactory: clientFactory, Cipher: options.Cipher, Clock: options.Clock,
		PostCommit: postCommit,
	})
	if err != nil {
		return nil, fmt.Errorf("initialize usage collection service: %w", err)
	}
	localRebuildService, err := service.NewLocalRebuildService(service.LocalRebuildServiceOptions{
		Database: options.Database.GORM, Clock: options.Clock,
	})
	if err != nil {
		return nil, fmt.Errorf("initialize local rebuild service: %w", err)
	}
	workerRuntime, err := worker.NewRuntime(worker.RuntimeOptions{
		Repository: model.NewCollectionTaskRepository(options.Database.GORM),
		Settings:   model.NewCollectorSettingRepository(options.Database.GORM), Clock: options.Clock,
		SiteJobs: siteService, UsageCollector: usageCollectionService,
		LogCollector: logService,
		PostCommit:   postCommit,
		Handlers:     worker.LocalRebuildJobHandlers(localRebuildService),
		Metrics:      metrics, MetricsRepository: model.NewOperationalMetricsRepository(options.Database.GORM),
		MetricsDatabase: options.Database.SQL, ExportDir: options.Config.ExportDir,
		RequiredTaskTypes: []string{
			constant.TaskTypeSiteProbe, constant.TaskTypeRealtimeStat, constant.TaskTypeResourceSnapshot,
			constant.TaskTypeUserSync, constant.TaskTypeChannelSync, constant.TaskTypeUsageHour,
			constant.TaskTypePerformanceSync,
			constant.TaskTypeTopupSync, constant.TaskTypeRedemptionSync, constant.TaskTypeUpstreamTaskSync,
			constant.TaskTypeModelMetaSync,
			constant.TaskTypePlanSync,
			constant.TaskTypePricingSync,
			constant.TaskTypeSystemTaskSync,
			constant.TaskTypeLogSync,
			constant.TaskTypeUsageBackfill, constant.TaskTypeUsageValidation,
			constant.TaskTypeAccountRebuild, constant.TaskTypeCustomerRebuild,
		},
		FastTaskHistory: redisStore,
	})
	if err != nil {
		return nil, fmt.Errorf("initialize worker runtime: %w", err)
	}
	resourceRetentionService, err := service.NewResourceRetentionService(service.ResourceRetentionServiceOptions{
		Repository: model.NewResourceRetentionRepository(options.Database.GORM), Clock: options.Clock,
	})
	if err != nil {
		return nil, fmt.Errorf("initialize resource retention service: %w", err)
	}
	resourceRetentionRuntime, err := worker.NewResourceRetentionRuntime(worker.ResourceRetentionRuntimeOptions{
		Cleaner:  resourceRetentionService,
		Settings: model.NewCollectorSettingRepository(options.Database.GORM),
		Clock:    options.Clock,
	})
	if err != nil {
		return nil, fmt.Errorf("initialize resource retention runtime: %w", err)
	}
	logRetentionService, err := service.NewUpstreamLogRetentionService(model.NewUpstreamLogRepository(options.Database.GORM), options.Clock)
	if err != nil {
		return nil, fmt.Errorf("initialize upstream log retention service: %w", err)
	}
	logRetentionRuntime, err := worker.NewUpstreamLogRetentionRuntime(logRetentionService, options.Clock)
	if err != nil {
		return nil, fmt.Errorf("initialize upstream log retention runtime: %w", err)
	}
	exportRuntime, err := worker.NewExportRuntime(worker.ExportRuntimeOptions{
		Database: options.Database.GORM, Clock: options.Clock, ExportDir: options.Config.ExportDir, Metrics: metrics,
	})
	if err != nil {
		return nil, fmt.Errorf("initialize export runtime: %w", err)
	}
	maintenanceService, err := service.NewDataMaintenanceService(
		model.NewDataMaintenanceRepository(options.Database.GORM), options.Clock,
	)
	if err != nil {
		return nil, fmt.Errorf("initialize data maintenance service: %w", err)
	}
	maintenanceRuntime, err := worker.NewDataMaintenanceRuntime(maintenanceService, options.Clock, maintenanceWake, 0)
	if err != nil {
		return nil, fmt.Errorf("initialize data maintenance runtime: %w", err)
	}
	applicationRuntime, err := newRuntimeGroup(workerRuntime, alertRuntime, exportRuntime, resourceRetentionRuntime, logRetentionRuntime, maintenanceRuntime)
	if err != nil {
		return nil, fmt.Errorf("initialize application runtime: %w", err)
	}
	return applicationRuntime, nil
}

func (app *application) Start(ctx context.Context) error {
	if app == nil || app.runtime == nil || app.Readiness == nil {
		return errors.New("application is not initialized")
	}
	app.Readiness.SetInitialized(true)
	app.Readiness.SetSchedulerReady(false)
	if app.Metrics != nil {
		app.Metrics.SetReady(false)
		app.Metrics.SetRuntimeReady("application", false)
		app.Metrics.SetRuntimeReady("metrics", true)
	}
	if err := app.runtime.Start(ctx); err != nil {
		app.MarkNotReady()
		return fmt.Errorf("start worker runtime: %w", err)
	}
	if !app.runtime.Ready() {
		app.MarkNotReady()
		return errors.New("worker runtime did not become ready")
	}
	app.Readiness.SetSchedulerReady(true)
	if app.Metrics != nil {
		app.Metrics.SetReady(true)
		app.Metrics.SetRuntimeReady("application", true)
	}
	return nil
}

func (app *application) Stop(ctx context.Context) error {
	quiesceError := app.BeginShutdown()
	if app == nil || app.runtime == nil {
		return quiesceError
	}
	return errors.Join(quiesceError, app.runtime.Stop(ctx))
}

func (app *application) MarkNotReady() {
	if app == nil {
		return
	}
	if app.Readiness != nil {
		app.Readiness.SetInitialized(false)
		app.Readiness.SetSchedulerReady(false)
	}
	if app.Metrics != nil {
		app.Metrics.SetReady(false)
		app.Metrics.SetRuntimeReady("application", false)
		app.Metrics.SetRuntimeReady("metrics", false)
	}
}

func (app *application) Quiesce() error {
	if app == nil || app.runtime == nil {
		return nil
	}
	return app.runtime.Quiesce()
}

func (app *application) BeginShutdown() error {
	if app == nil {
		return nil
	}
	app.MarkNotReady()
	return app.Quiesce()
}

func (app *application) RuntimeReady() bool {
	return app != nil && app.runtime != nil && app.runtime.Ready()
}
