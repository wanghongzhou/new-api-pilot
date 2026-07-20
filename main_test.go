package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"new-api-pilot/common"
	"new-api-pilot/config"
	"new-api-pilot/model"
	"new-api-pilot/router"
)

func TestA49CapacityServeOptionsRequireExactAcceptanceGuard(t *testing.T) {
	fixedNow := "1752400800"
	tests := []struct {
		name   string
		appEnv string
		values map[string]string
	}{
		{name: "nil lookup", appEnv: config.EnvironmentTest},
		{name: "development", appEnv: config.EnvironmentDevelopment, values: map[string]string{"ACCEPTANCE_ID": a49AcceptanceID, a49FixedNowUnixEnv: fixedNow}},
		{name: "production", appEnv: config.EnvironmentProduction, values: map[string]string{"ACCEPTANCE_ID": a49AcceptanceID, a49FixedNowUnixEnv: fixedNow}},
		{name: "missing acceptance", appEnv: config.EnvironmentTest, values: map[string]string{a49FixedNowUnixEnv: fixedNow}},
		{name: "wrong acceptance", appEnv: config.EnvironmentTest, values: map[string]string{"ACCEPTANCE_ID": "A85", a49FixedNowUnixEnv: fixedNow}},
		{name: "acceptance whitespace", appEnv: config.EnvironmentTest, values: map[string]string{"ACCEPTANCE_ID": " A49", a49FixedNowUnixEnv: fixedNow}},
		{name: "missing clock", appEnv: config.EnvironmentTest, values: map[string]string{"ACCEPTANCE_ID": a49AcceptanceID}},
		{name: "empty clock", appEnv: config.EnvironmentTest, values: map[string]string{"ACCEPTANCE_ID": a49AcceptanceID, a49FixedNowUnixEnv: ""}},
		{name: "clock whitespace", appEnv: config.EnvironmentTest, values: map[string]string{"ACCEPTANCE_ID": a49AcceptanceID, a49FixedNowUnixEnv: " 1752400800"}},
		{name: "clock leading plus", appEnv: config.EnvironmentTest, values: map[string]string{"ACCEPTANCE_ID": a49AcceptanceID, a49FixedNowUnixEnv: "+1752400800"}},
		{name: "clock leading zero", appEnv: config.EnvironmentTest, values: map[string]string{"ACCEPTANCE_ID": a49AcceptanceID, a49FixedNowUnixEnv: "01752400800"}},
		{name: "clock zero", appEnv: config.EnvironmentTest, values: map[string]string{"ACCEPTANCE_ID": a49AcceptanceID, a49FixedNowUnixEnv: "0"}},
		{name: "clock negative", appEnv: config.EnvironmentTest, values: map[string]string{"ACCEPTANCE_ID": a49AcceptanceID, a49FixedNowUnixEnv: "-1"}},
		{name: "clock beyond year 9999", appEnv: config.EnvironmentTest, values: map[string]string{"ACCEPTANCE_ID": a49AcceptanceID, a49FixedNowUnixEnv: "253402300800"}},
		{name: "clock overflow", appEnv: config.EnvironmentTest, values: map[string]string{"ACCEPTANCE_ID": a49AcceptanceID, a49FixedNowUnixEnv: "9223372036854775808"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var lookup config.LookupFunc
			if test.values != nil {
				lookup = func(key string) (string, bool) {
					value, exists := test.values[key]
					return value, exists
				}
			}
			if _, err := a49CapacityServeOptions(config.Config{AppEnv: test.appEnv}, lookup); err == nil {
				t.Fatal("invalid A49 capacity environment was accepted")
			}
		})
	}
}

func TestA49CapacityServeOptionsBuildFixedReadOnlyRuntime(t *testing.T) {
	const fixedNow = int64(1752400800)
	values := map[string]string{
		"ACCEPTANCE_ID":    a49AcceptanceID,
		a49FixedNowUnixEnv: fmt.Sprintf("%d", fixedNow),
	}
	options, err := a49CapacityServeOptions(config.Config{AppEnv: config.EnvironmentTest}, func(key string) (string, bool) {
		value, exists := values[key]
		return value, exists
	})
	if err != nil {
		t.Fatalf("build A49 capacity serve options: %v", err)
	}
	if options.runtimeMode != applicationRuntimeA49ReadOnly || options.acceptanceID != a49AcceptanceID {
		t.Fatalf("A49 capacity options = %#v", options)
	}
	if got := options.clock.Now().Unix(); got != fixedNow {
		t.Fatalf("fixed clock Now() = %d, want %d", got, fixedNow)
	}
	time.Sleep(time.Millisecond)
	if got := options.clock.Now().Unix(); got != fixedNow {
		t.Fatalf("fixed clock advanced to %d, want %d", got, fixedNow)
	}
}

func TestApplicationRuntimeOptionsFailClosed(t *testing.T) {
	tests := []applicationOptions{
		{Config: config.Config{AppEnv: config.EnvironmentTest}, RuntimeMode: applicationRuntimeStandard, AcceptanceID: a49AcceptanceID},
		{Config: config.Config{AppEnv: config.EnvironmentProduction}, RuntimeMode: applicationRuntimeA49ReadOnly, AcceptanceID: a49AcceptanceID},
		{Config: config.Config{AppEnv: config.EnvironmentTest}, RuntimeMode: applicationRuntimeA49ReadOnly, AcceptanceID: "A85"},
		{Config: config.Config{AppEnv: config.EnvironmentTest}, RuntimeMode: applicationRuntimeMode(255)},
	}
	for index, options := range tests {
		if err := validateApplicationRuntimeOptions(options); err == nil {
			t.Fatalf("invalid runtime options %d were accepted: %#v", index, options)
		}
	}
	if err := validateApplicationRuntimeOptions(applicationOptions{
		Config: config.Config{AppEnv: config.EnvironmentTest}, RuntimeMode: applicationRuntimeA49ReadOnly,
		AcceptanceID: a49AcceptanceID,
	}); err != nil {
		t.Fatalf("valid A49 runtime options: %v", err)
	}
}

func TestA49ReadOnlyRuntimeDoesNotRequireWorkerDependencies(t *testing.T) {
	runtime, err := buildApplicationRuntime(applicationOptions{RuntimeMode: applicationRuntimeA49ReadOnly},
		nil, nil, nil, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("build A49 read-only runtime: %v", err)
	}
	if _, ok := runtime.(*acceptanceReadOnlyRuntime); !ok {
		t.Fatalf("A49 runtime type = %T", runtime)
	}
	if runtime.Ready() {
		t.Fatal("unstarted A49 runtime reported ready")
	}
	if err := runtime.Start(context.Background()); err != nil || !runtime.Ready() {
		t.Fatalf("start A49 runtime ready=%v err=%v", runtime.Ready(), err)
	}
	if err := runtime.Quiesce(); err != nil || runtime.Ready() {
		t.Fatalf("quiesce A49 runtime ready=%v err=%v", runtime.Ready(), err)
	}
}

func TestApplicationLifecycleControlsReadiness(t *testing.T) {
	readiness := common.NewReadiness()
	metrics := common.NewMetrics()
	engine, err := router.New(router.Options{
		Config: config.Config{AppEnv: config.EnvironmentTest}, Readiness: readiness, Metrics: metrics,
	})
	if err != nil {
		t.Fatalf("create lifecycle router: %v", err)
	}
	runtime := &fakeRuntimeLifecycle{readyOnStart: true}
	app := &application{Handler: engine, Readiness: readiness, Metrics: metrics, runtime: runtime}

	assertApplicationReadiness(t, app.Handler, http.StatusServiceUnavailable)
	if err := app.Start(context.Background()); err != nil {
		t.Fatalf("start application: %v", err)
	}
	assertApplicationReadiness(t, app.Handler, http.StatusOK)
	if err := app.Stop(context.Background()); err != nil {
		t.Fatalf("stop application: %v", err)
	}
	assertApplicationReadiness(t, app.Handler, http.StatusServiceUnavailable)
	if runtime.startCalls != 1 || runtime.stopCalls != 1 {
		t.Fatalf("runtime calls start=%d stop=%d", runtime.startCalls, runtime.stopCalls)
	}
}

func TestApplicationStartFailureRemainsNotReady(t *testing.T) {
	readiness := common.NewReadiness()
	engine, err := router.New(router.Options{
		Config: config.Config{AppEnv: config.EnvironmentTest}, Readiness: readiness,
	})
	if err != nil {
		t.Fatalf("create failed-start router: %v", err)
	}
	startError := errors.New("injected runtime startup failure")
	runtime := &fakeRuntimeLifecycle{startErr: startError}
	app := &application{Handler: engine, Readiness: readiness, runtime: runtime}

	if err := app.Start(context.Background()); !errors.Is(err, startError) {
		t.Fatalf("start error = %v, want injected failure", err)
	}
	assertApplicationReadiness(t, app.Handler, http.StatusServiceUnavailable)
	if runtime.stopCalls != 0 {
		t.Fatalf("failed runtime stop calls = %d, want 0", runtime.stopCalls)
	}
}

func TestDatabaseInitializationMigrationFailureKeepsReadinessClosedAndSkipsSeed(t *testing.T) {
	readiness := common.NewReadiness()
	engine, err := router.New(router.Options{
		Config: config.Config{AppEnv: config.EnvironmentTest}, Readiness: readiness,
	})
	if err != nil {
		t.Fatalf("create migration-gated router: %v", err)
	}
	migrationError := fmt.Errorf("%w: injected unknown version", model.ErrMigrationSourceInvalid)
	migrationStep := &fakeDatabaseBootstrapStep{err: migrationError}
	seedStep := &fakeDatabaseBootstrapStep{}

	err = initializeDatabase(context.Background(), migrationStep, seedStep)
	if !errors.Is(err, model.ErrMigrationSourceInvalid) {
		t.Fatalf("database initialization error = %v", err)
	}
	if migrationStep.calls != 1 || seedStep.calls != 0 {
		t.Fatalf("database bootstrap calls migration=%d seed=%d", migrationStep.calls, seedStep.calls)
	}
	assertApplicationReadiness(t, engine, http.StatusServiceUnavailable)
}

func TestShutdownStopsAdmissionAndQuiescesBeforeHTTPDrain(t *testing.T) {
	readiness := common.NewReadiness()
	readiness.SetInitialized(true)
	readiness.SetSchedulerReady(true)
	quiesced := make(chan struct{})
	stopStarted := make(chan struct{})
	stopRelease := make(chan struct{})
	stopped := make(chan struct{})
	runtime := &fakeRuntimeLifecycle{
		ready: true, quiesced: quiesced, stopStarted: stopStarted, stopRelease: stopRelease, stopped: stopped,
	}
	requestEntered := make(chan struct{})
	releaseRequest := make(chan struct{})
	handler := http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		close(requestEntered)
		<-releaseRequest
		response.WriteHeader(http.StatusNoContent)
	})
	app := &application{Handler: handler, Readiness: readiness, runtime: runtime}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen for shutdown test: %v", err)
	}
	address := listener.Addr().String()
	server := &http.Server{Handler: handler}
	serveDone := make(chan error, 1)
	go func() { serveDone <- server.Serve(listener) }()
	responseDone := make(chan error, 1)
	go func() {
		response, requestErr := http.Get("http://" + address)
		if requestErr == nil {
			_ = response.Body.Close()
		}
		responseDone <- requestErr
	}()
	select {
	case <-requestEntered:
	case <-time.After(time.Second):
		t.Fatal("active HTTP request did not enter handler")
	}
	shutdownDone := make(chan error, 1)
	go func() { shutdownDone <- shutdownApplication(app, server, listener, 2*time.Second) }()
	select {
	case <-quiesced:
	case <-time.After(time.Second):
		t.Fatal("runtime was not quiesced")
	}
	select {
	case <-stopStarted:
	case <-time.After(time.Second):
		t.Fatal("runtime drain did not start alongside HTTP drain")
	}
	if failures := readiness.Check(context.Background()); len(failures) == 0 {
		t.Fatal("readiness remained healthy during shutdown")
	}
	connection, dialErr := net.DialTimeout("tcp", address, 100*time.Millisecond)
	if dialErr == nil {
		_ = connection.Close()
		t.Fatal("listener accepted a new connection after quiesce")
	}
	close(releaseRequest)
	select {
	case err := <-responseDone:
		if err != nil {
			t.Fatalf("active HTTP request failed during drain: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("active HTTP request did not drain")
	}
	close(stopRelease)
	select {
	case err := <-shutdownDone:
		if err != nil {
			t.Fatalf("shutdown application: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("application shutdown did not finish")
	}
	select {
	case <-stopped:
	default:
		t.Fatal("runtime was not stopped after HTTP drain")
	}
	select {
	case <-serveDone:
	case <-time.After(time.Second):
		t.Fatal("HTTP server did not stop")
	}
}

func TestShutdownReturnsAtHardDeadlineWhenWorkIgnoresCancellation(t *testing.T) {
	readiness := common.NewReadiness()
	readiness.SetInitialized(true)
	readiness.SetSchedulerReady(true)
	runtime := &ignoringStopRuntime{
		stopStarted: make(chan struct{}), stopRelease: make(chan struct{}), stopped: make(chan struct{}),
	}
	requestStarted := make(chan struct{})
	requestRelease := make(chan struct{})
	requestStopped := make(chan struct{})
	handler := http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		close(requestStarted)
		<-requestRelease
		close(requestStopped)
		response.WriteHeader(http.StatusNoContent)
	})
	app := &application{Handler: handler, Readiness: readiness, runtime: runtime}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen for hard shutdown test: %v", err)
	}
	server := &http.Server{Handler: handler}
	serveDone := make(chan error, 1)
	go func() { serveDone <- server.Serve(listener) }()
	requestDone := make(chan error, 1)
	go func() {
		response, requestErr := http.Get("http://" + listener.Addr().String())
		if requestErr == nil {
			_ = response.Body.Close()
		}
		requestDone <- requestErr
	}()
	select {
	case <-requestStarted:
	case <-time.After(time.Second):
		t.Fatal("hard shutdown request did not start")
	}

	startedAt := time.Now()
	err = shutdownApplication(app, server, listener, 150*time.Millisecond)
	elapsed := time.Since(startedAt)
	if !errors.Is(err, errApplicationShutdownTimeout) || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("hard shutdown error = %v", err)
	}
	if elapsed < 100*time.Millisecond || elapsed > 500*time.Millisecond {
		t.Fatalf("hard shutdown elapsed = %v", elapsed)
	}
	select {
	case <-runtime.stopStarted:
	default:
		t.Fatal("runtime stop did not start")
	}

	close(requestRelease)
	close(runtime.stopRelease)
	select {
	case <-requestStopped:
	case <-time.After(time.Second):
		t.Fatal("ignored HTTP handler did not clean up after release")
	}
	select {
	case <-runtime.stopped:
	case <-time.After(time.Second):
		t.Fatal("ignored runtime did not clean up after release")
	}
	select {
	case <-requestDone:
	case <-time.After(time.Second):
		t.Fatal("hard shutdown client did not return")
	}
	select {
	case <-serveDone:
	case <-time.After(time.Second):
		t.Fatal("hard shutdown server did not return")
	}
}

type fakeRuntimeLifecycle struct {
	readyOnStart bool
	ready        bool
	startErr     error
	stopErr      error
	startCalls   int
	stopCalls    int
	quiesceCalls int
	quiesced     chan struct{}
	stopStarted  chan struct{}
	stopRelease  chan struct{}
	stopped      chan struct{}
}

type fakeDatabaseBootstrapStep struct {
	err   error
	calls int
}

func (step *fakeDatabaseBootstrapStep) Run(context.Context) error {
	step.calls++
	return step.err
}

type ignoringStopRuntime struct {
	stopStarted chan struct{}
	stopRelease chan struct{}
	stopped     chan struct{}
}

func (runtime *ignoringStopRuntime) Start(context.Context) error { return nil }
func (runtime *ignoringStopRuntime) Quiesce() error              { return nil }
func (runtime *ignoringStopRuntime) Ready() bool                 { return true }
func (runtime *ignoringStopRuntime) Stop(context.Context) error {
	close(runtime.stopStarted)
	<-runtime.stopRelease
	close(runtime.stopped)
	return nil
}

func (runtime *fakeRuntimeLifecycle) Start(context.Context) error {
	runtime.startCalls++
	if runtime.startErr != nil {
		return runtime.startErr
	}
	runtime.ready = runtime.readyOnStart
	return nil
}

func (runtime *fakeRuntimeLifecycle) Stop(ctx context.Context) error {
	runtime.stopCalls++
	if runtime.stopStarted != nil && runtime.stopCalls == 1 {
		close(runtime.stopStarted)
	}
	if runtime.stopRelease != nil {
		select {
		case <-runtime.stopRelease:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	runtime.ready = false
	if runtime.stopped != nil && runtime.stopCalls == 1 {
		close(runtime.stopped)
	}
	return runtime.stopErr
}

func (runtime *fakeRuntimeLifecycle) Quiesce() error {
	runtime.quiesceCalls++
	runtime.ready = false
	if runtime.quiesced != nil && runtime.quiesceCalls == 1 {
		close(runtime.quiesced)
	}
	return nil
}

func (runtime *fakeRuntimeLifecycle) Ready() bool { return runtime.ready }

func assertApplicationReadiness(t *testing.T, handler http.Handler, expected int) {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != expected {
		t.Fatalf("readiness status = %d, want %d, body=%s", response.Code, expected, response.Body.String())
	}
}
