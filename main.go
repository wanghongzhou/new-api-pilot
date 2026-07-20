package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"new-api-pilot/common"
	"new-api-pilot/config"
	"new-api-pilot/model"
)

var errApplicationShutdownTimeout = errors.New("application shutdown deadline exceeded")

const (
	a49AcceptanceID      = "A49"
	a49FixedNowUnixEnv   = "A49_FIXED_NOW_UNIX"
	maximumSupportedUnix = int64(253402300799)
)

type serveOptions struct {
	clock        common.Clock
	runtimeMode  applicationRuntimeMode
	acceptanceID string
}

type fixedNowClock struct {
	common.SystemClock
	now time.Time
}

func (clock fixedNowClock) Now() time.Time { return clock.now }

type databaseBootstrapStep interface {
	Run(context.Context) error
}

func main() {
	if isMaintenanceCommand(os.Args[1:]) {
		os.Exit(runMaintenanceCommand(os.Args[1:], os.Stdout))
	}
	if err := run(os.Args[1:]); err != nil {
		log.Fatal(err)
	}
}

func run(args []string) error {
	appConfig, err := config.Load()
	if err != nil {
		return fmt.Errorf("load configuration: %w", err)
	}
	if len(args) > 0 {
		switch args[0] {
		case "migrate":
			return migrate(appConfig)
		case "capacity-serve":
			return serveA49Capacity(appConfig, os.LookupEnv)
		default:
			return fmt.Errorf("unknown command %q", args[0])
		}
	}
	return serve(appConfig)
}

func migrate(appConfig config.Config) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	database, err := openDatabase(ctx, appConfig)
	if err != nil {
		return err
	}
	defer func() { _ = database.Close() }()
	if err := model.NewMigrationRunner(database.SQL).Run(ctx); err != nil {
		return fmt.Errorf("run migrations: %w", err)
	}
	log.Print("database migrations complete")
	return nil
}

func serve(appConfig config.Config) error {
	return serveWithOptions(appConfig, serveOptions{clock: common.SystemClock{}})
}

func serveA49Capacity(appConfig config.Config, lookup config.LookupFunc) error {
	options, err := a49CapacityServeOptions(appConfig, lookup)
	if err != nil {
		return err
	}
	return serveWithOptions(appConfig, options)
}

func a49CapacityServeOptions(appConfig config.Config, lookup config.LookupFunc) (serveOptions, error) {
	if lookup == nil {
		return serveOptions{}, errors.New("A49 capacity serve environment lookup is required")
	}
	if appConfig.AppEnv != config.EnvironmentTest {
		return serveOptions{}, errors.New("capacity-serve is restricted to APP_ENV=test")
	}
	acceptanceID, exists := lookup("ACCEPTANCE_ID")
	if !exists || acceptanceID != a49AcceptanceID {
		return serveOptions{}, errors.New("capacity-serve requires ACCEPTANCE_ID=A49")
	}
	rawNow, exists := lookup(a49FixedNowUnixEnv)
	if !exists || rawNow == "" {
		return serveOptions{}, fmt.Errorf("capacity-serve requires %s", a49FixedNowUnixEnv)
	}
	fixedNow, err := strconv.ParseInt(rawNow, 10, 64)
	if err != nil || strconv.FormatInt(fixedNow, 10) != rawNow || fixedNow <= 0 || fixedNow > maximumSupportedUnix {
		return serveOptions{}, fmt.Errorf("%s must be a canonical Unix timestamp in the supported range", a49FixedNowUnixEnv)
	}
	return serveOptions{
		clock:        fixedNowClock{now: time.Unix(fixedNow, 0)},
		runtimeMode:  applicationRuntimeA49ReadOnly,
		acceptanceID: acceptanceID,
	}, nil
}

func serveWithOptions(appConfig config.Config, options serveOptions) error {
	if options.clock == nil {
		return errors.New("serve clock is required")
	}
	if err := appConfig.ValidateRuntimeFiles(); err != nil {
		return fmt.Errorf("validate runtime files: %w", err)
	}
	cipher, err := common.NewCipher(appConfig.EncryptionKey)
	if err != nil {
		return fmt.Errorf("initialize encryption: %w", err)
	}

	startupContext, cancelStartup := context.WithTimeout(context.Background(), 2*time.Minute)
	database, err := openDatabase(startupContext, appConfig)
	if err != nil {
		cancelStartup()
		return err
	}
	if err := initializeDatabase(
		startupContext,
		model.NewMigrationRunner(database.SQL),
		model.NewSeeder(database.SQL),
	); err != nil {
		cancelStartup()
		_ = database.Close()
		return err
	}
	app, bootstrap, err := bootstrapApplication(startupContext, applicationOptions{
		Config: appConfig, Database: database, Cipher: cipher, Clock: options.clock,
		RuntimeMode: options.runtimeMode, AcceptanceID: options.acceptanceID,
	})
	cancelStartup()
	if err != nil {
		_ = database.Close()
		return err
	}
	if bootstrap.Created && bootstrap.GeneratedPassword != "" {
		fmt.Printf("bootstrap admin username=admin password=%s\n", bootstrap.GeneratedPassword)
	}
	defer func() { _ = database.Close() }()

	runtimeContext, cancelRuntime := context.WithCancel(context.Background())
	defer cancelRuntime()
	if err := app.Start(runtimeContext); err != nil {
		return err
	}

	server := &http.Server{
		Addr:              ":" + appConfig.Port,
		Handler:           app.Handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      120 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	listener, err := net.Listen("tcp", server.Addr)
	if err != nil {
		stopContext, cancelStop := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancelStop()
		return errors.Join(fmt.Errorf("listen HTTP: %w", err), app.Stop(stopContext))
	}
	serveError := make(chan error, 1)
	go func() {
		log.Printf("new-api-pilot listening port=%s encryption_key_id=%s", appConfig.Port, appConfig.EncryptionKeyID)
		err := server.Serve(listener)
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
		serveError <- err
	}()
	runtimeFailure := monitorRuntime(app)

	shutdownSignal := make(chan os.Signal, 1)
	signal.Notify(shutdownSignal, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(shutdownSignal)
	var terminalError error
	select {
	case err := <-serveError:
		if err != nil {
			terminalError = fmt.Errorf("serve HTTP: %w", err)
		}
	case err := <-runtimeFailure:
		terminalError = err
	case <-shutdownSignal:
	}

	shutdownError := shutdownApplication(app, server, listener, 30*time.Second)
	return errors.Join(terminalError, shutdownError)
}

func initializeDatabase(
	ctx context.Context,
	migrationStep databaseBootstrapStep,
	seedStep databaseBootstrapStep,
) error {
	if ctx == nil || migrationStep == nil || seedStep == nil {
		return errors.New("database bootstrap dependencies are required")
	}
	if err := migrationStep.Run(ctx); err != nil {
		return fmt.Errorf("run migrations: %w", err)
	}
	if err := seedStep.Run(ctx); err != nil {
		return fmt.Errorf("seed defaults: %w", err)
	}
	return nil
}

func shutdownApplication(
	app *application,
	server *http.Server,
	listener net.Listener,
	timeout time.Duration,
) error {
	if app == nil || server == nil || listener == nil || timeout <= 0 {
		return errors.New("shutdown dependencies are required")
	}
	deadline := time.Now().Add(timeout)
	app.MarkNotReady()
	listenerCloseError := listener.Close()
	if errors.Is(listenerCloseError, net.ErrClosed) {
		listenerCloseError = nil
	}
	quiesceError := app.Quiesce()
	forceJoinReserve := timeout / 5
	if forceJoinReserve > 5*time.Second {
		forceJoinReserve = 5 * time.Second
	}
	gracefulDeadline := deadline.Add(-forceJoinReserve)
	drainContext, cancelDrain := context.WithDeadline(context.Background(), gracefulDeadline)
	defer cancelDrain()
	hardContext, cancelHard := context.WithDeadline(context.Background(), deadline)
	defer cancelHard()

	type shutdownResult struct {
		http bool
		err  error
	}
	results := make(chan shutdownResult, 2)
	go func() {
		err := server.Shutdown(drainContext)
		if err != nil && drainContext.Err() != nil {
			err = errors.Join(err, server.Close())
		}
		results <- shutdownResult{http: true, err: err}
	}()
	go func() {
		results <- shutdownResult{err: app.Stop(hardContext)}
	}()

	var shutdownError error
	var stopError error
	for range 2 {
		select {
		case result := <-results:
			if result.http {
				shutdownError = result.err
			} else {
				stopError = result.err
			}
		case <-hardContext.Done():
			shutdownError = errors.Join(shutdownError, server.Close())
			stopError = errors.Join(stopError, errApplicationShutdownTimeout, hardContext.Err())
			return formatShutdownErrors(listenerCloseError, quiesceError, stopError, shutdownError)
		}
	}
	return formatShutdownErrors(listenerCloseError, quiesceError, stopError, shutdownError)
}

func formatShutdownErrors(listenerCloseError, quiesceError, stopError, shutdownError error) error {
	if quiesceError != nil {
		quiesceError = fmt.Errorf("quiesce application runtime: %w", quiesceError)
	}
	if stopError != nil {
		stopError = fmt.Errorf("stop application runtime: %w", stopError)
	}
	if shutdownError != nil {
		shutdownError = fmt.Errorf("shutdown HTTP server: %w", shutdownError)
	}
	if listenerCloseError != nil {
		listenerCloseError = fmt.Errorf("close HTTP listener: %w", listenerCloseError)
	}
	return errors.Join(listenerCloseError, quiesceError, stopError, shutdownError)
}

func monitorRuntime(app *application) <-chan error {
	failure := make(chan error, 1)
	go func() {
		ticker := time.NewTicker(250 * time.Millisecond)
		defer ticker.Stop()
		for range ticker.C {
			if !app.RuntimeReady() {
				failure <- errors.New("worker runtime stopped unexpectedly")
				return
			}
		}
	}()
	return failure
}

func openDatabase(ctx context.Context, appConfig config.Config) (*model.Database, error) {
	database, err := model.Open(ctx, model.Options{
		DSN:         appConfig.DatabaseDSN,
		MaxIdle:     appConfig.SQLMaxIdleConns,
		MaxOpen:     appConfig.SQLMaxOpenConns,
		MaxLifetime: appConfig.SQLMaxLifetime,
	})
	if err != nil {
		return nil, fmt.Errorf("initialize database: %w", err)
	}
	return database, nil
}
