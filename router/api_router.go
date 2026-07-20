package router

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"new-api-pilot/common"
	"new-api-pilot/config"
	"new-api-pilot/constant"
	"new-api-pilot/controller"
	"new-api-pilot/middleware"
)

type Options struct {
	Config                       config.Config
	Database                     *sql.DB
	Readiness                    *common.Readiness
	Metrics                      *common.Metrics
	AuthController               *controller.AuthController
	UserController               *controller.PlatformUserController
	SiteController               *controller.SiteController
	CustomerController           *controller.CustomerController
	AccountController            *controller.AccountController
	StatisticsController         *controller.StatisticsController
	ExportController             *controller.ExportController
	DashboardController          *controller.DashboardController
	AlertController              *controller.AlertController
	SettingController            *controller.SettingController
	NotificationController       *controller.NotificationController
	FastTaskController           *controller.FastTaskController
	LogController                *controller.LogController
	UserInventoryController      *controller.UserInventoryController
	ChannelInventoryController   *controller.ChannelInventoryController
	PerformanceHistoryController *controller.PerformanceHistoryController
	FinanceOperationsController  *controller.FinanceOperationsController
	UpstreamTaskController       *controller.UpstreamTaskController
	ModelCatalogController       *controller.ModelCatalogController
	LocalRankingController       *controller.LocalRankingController
	SubscriptionPlanController   *controller.SubscriptionPlanController
	PricingCatalogController     *controller.PricingCatalogController
	SystemTaskCatalogController  *controller.SystemTaskCatalogController
	IdentityResolver             middleware.IdentityResolver
	WebAssets                    fs.FS
}

func New(options Options) (*gin.Engine, error) {
	if options.Config.AppEnv == config.EnvironmentProduction {
		gin.SetMode(gin.ReleaseMode)
	}
	engine := gin.New()
	if err := engine.SetTrustedProxies(options.Config.TrustedProxies); err != nil {
		return nil, err
	}
	static, err := newStaticServer(options.WebAssets)
	if err != nil {
		return nil, fmt.Errorf("initialize static web server: %w", err)
	}
	engine.Use(
		middleware.RequestID(),
		middleware.AccessLog(),
		middleware.Recovery(),
		middleware.SecurityHeaders(),
		middleware.OriginGuard(options.Config.AppEnv, options.Config.PublicOrigin),
	)
	if options.Metrics != nil {
		engine.Use(middleware.HTTPMetrics(options.Metrics))
	}
	registerUserRoutes(engine, options.AuthController, options.UserController, options.IdentityResolver)
	registerSiteRoutes(engine, options.SiteController, options.IdentityResolver)
	registerCustomerRoutes(engine, options.CustomerController, options.AccountController, options.IdentityResolver)
	RegisterStatisticsRoutes(engine, options.StatisticsController, options.IdentityResolver)
	RegisterLogRoutes(engine, options.LogController, options.IdentityResolver)
	RegisterUserInventoryRoutes(engine, options.UserInventoryController, options.IdentityResolver)
	RegisterChannelInventoryRoutes(engine, options.ChannelInventoryController, options.IdentityResolver)
	RegisterPerformanceHistoryRoutes(engine, options.PerformanceHistoryController, options.IdentityResolver)
	RegisterFinanceOperationsRoutes(engine, options.FinanceOperationsController, options.IdentityResolver)
	RegisterUpstreamTaskRoutes(engine, options.UpstreamTaskController, options.IdentityResolver)
	RegisterModelCatalogRoutes(engine, options.ModelCatalogController, options.IdentityResolver)
	RegisterLocalRankingRoutes(engine, options.LocalRankingController, options.IdentityResolver)
	RegisterSubscriptionPlanRoutes(engine, options.SubscriptionPlanController, options.IdentityResolver)
	RegisterPricingCatalogRoutes(engine, options.PricingCatalogController, options.IdentityResolver)
	RegisterSystemTaskCatalogRoutes(engine, options.SystemTaskCatalogController, options.IdentityResolver)
	RegisterExportRoutes(engine, options.ExportController, options.IdentityResolver)
	RegisterDashboardRoutes(engine, options.DashboardController, options.IdentityResolver)
	RegisterAlertRoutes(engine, options.AlertController, options.IdentityResolver)
	RegisterSettingRoutes(engine, options.SettingController, options.IdentityResolver)
	RegisterNotificationRoutes(engine, options.NotificationController, options.IdentityResolver)
	if options.FastTaskController != nil {
		engine.GET("/api/fast-tasks", middleware.UserAuth(options.IdentityResolver), options.FastTaskController.List)
	}

	engine.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	engine.GET("/readyz", readinessHandler(options.Readiness, options.Metrics))
	if options.Metrics != nil {
		metricsHandlers := []gin.HandlerFunc{middleware.AllowCIDRs(options.Config.MetricsAllowedCIDRs)}
		metricsHandlers = append(metricsHandlers, func(c *gin.Context) {
			ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
			defer cancel()
			options.Metrics.SetReady(len(readinessFailures(ctx, options.Readiness)) == 0)
			if options.Database != nil {
				options.Metrics.SetDBStats(options.Database.Stats())
			}
			gin.WrapH(options.Metrics.Handler())(c)
		})
		engine.GET("/metrics", metricsHandlers...)
	}

	engine.NoRoute(func(c *gin.Context) {
		if strings.HasPrefix(c.Request.URL.Path, "/api") {
			common.WriteError(c, http.StatusNotFound, constant.CodeNotFound, "Resource not found", nil)
			return
		}
		if isOperationalPath(c.Request.URL.Path) {
			c.JSON(http.StatusNotFound, gin.H{"status": "not_found"})
			return
		}
		if static != nil && (c.Request.Method == http.MethodGet || c.Request.Method == http.MethodHead) {
			name, safe := staticAssetName(c.Request.URL.Path)
			if safe {
				static.serve(c, name)
				return
			}
		}
		c.JSON(http.StatusNotFound, gin.H{"status": "not_found"})
	})
	return engine, nil
}

func readinessHandler(readiness *common.Readiness, metrics *common.Metrics) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		defer cancel()
		failures := readinessFailures(ctx, readiness)
		ready := len(failures) == 0
		if metrics != nil {
			metrics.SetReady(ready)
		}
		if !ready {
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "not_ready", "failed_checks": failures})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ready"})
	}
}

func readinessFailures(ctx context.Context, readiness *common.Readiness) []string {
	if readiness == nil {
		return []string{"runtime"}
	}
	return readiness.Check(ctx)
}
