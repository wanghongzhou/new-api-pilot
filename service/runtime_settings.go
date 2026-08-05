package service

import (
	"context"
	"errors"
	"log"
	"net/netip"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/net/idna"

	"new-api-pilot/common"
	"new-api-pilot/model"
)

const runtimeSettingsRefreshInterval = time.Second

type RuntimeSettingsSnapshot struct {
	FastTaskRetention time.Duration
	FastTaskCount     int
	AllowedHosts      []string
	AllowedCIDRs      []netip.Prefix
	ConnectTimeout    time.Duration
	HeaderTimeout     time.Duration
	RequestTimeout    time.Duration
	ExportTimeout     time.Duration
	Governor          UpstreamGovernor
	governorOptions   UpstreamGovernorOptions
}

type RuntimeSettingsStore struct {
	clock        common.Clock
	repository   *model.SettingRepository
	pollInterval time.Duration
	value        atomic.Pointer[RuntimeSettingsSnapshot]
	version      atomic.Int64
	refreshMu    sync.Mutex
	publishMu    sync.Mutex
	lifecycleMu  sync.Mutex
	cancel       context.CancelFunc
	done         chan struct{}
	ready        atomic.Bool
}

func LoadRuntimeSettingsStore(
	ctx context.Context,
	repository *model.SettingRepository,
	clock common.Clock,
) (*RuntimeSettingsStore, error) {
	if repository == nil || clock == nil {
		return nil, errors.New("runtime settings dependencies are required")
	}
	rows, err := repository.List(ctx, settingKeys())
	if err != nil {
		return nil, err
	}
	indexed, err := validateSettingRows(rows)
	if err != nil {
		return nil, err
	}
	values := make(map[string]string, len(indexed))
	for key, row := range indexed {
		values[key] = row.Value
	}
	snapshot, err := runtimeSettingsFromValues(values, clock)
	if err != nil {
		return nil, err
	}
	store := &RuntimeSettingsStore{clock: clock, repository: repository, pollInterval: runtimeSettingsRefreshInterval}
	if err := store.StoreVersioned(snapshot, runtimeSettingsVersion(indexed)); err != nil {
		return nil, err
	}
	return store, nil
}

func (store *RuntimeSettingsStore) Snapshot() RuntimeSettingsSnapshot {
	if store == nil || store.value.Load() == nil {
		return RuntimeSettingsSnapshot{}
	}
	snapshot := *store.value.Load()
	snapshot.AllowedHosts = append([]string(nil), snapshot.AllowedHosts...)
	snapshot.AllowedCIDRs = append([]netip.Prefix(nil), snapshot.AllowedCIDRs...)
	return snapshot
}

func (store *RuntimeSettingsStore) Store(snapshot RuntimeSettingsSnapshot) error {
	if store == nil {
		return errors.New("runtime settings store is required")
	}
	return store.StoreVersioned(snapshot, store.version.Load())
}

func (store *RuntimeSettingsStore) StoreVersioned(snapshot RuntimeSettingsSnapshot, version int64) error {
	if store == nil {
		return errors.New("runtime settings store is required")
	}
	store.publishMu.Lock()
	defer store.publishMu.Unlock()
	if version < store.version.Load() {
		return nil
	}
	if current := store.value.Load(); current != nil && current.Governor != nil {
		if err := current.Governor.Reconfigure(snapshot.governorOptions); err != nil {
			return err
		}
		snapshot.Governor = current.Governor
	}
	copyValue := snapshot
	copyValue.AllowedHosts = append([]string(nil), snapshot.AllowedHosts...)
	copyValue.AllowedCIDRs = append([]netip.Prefix(nil), snapshot.AllowedCIDRs...)
	store.value.Store(&copyValue)
	store.version.Store(version)
	return nil
}

func (store *RuntimeSettingsStore) Build(values map[string]string) (RuntimeSettingsSnapshot, error) {
	var governor UpstreamGovernor
	if current := store.value.Load(); current != nil {
		governor = current.Governor
	}
	return runtimeSettingsFromValues(values, store.clock, governor)
}

func (store *RuntimeSettingsStore) FastTaskHistorySettings() (time.Duration, int) {
	snapshot := store.Snapshot()
	return snapshot.FastTaskRetention, snapshot.FastTaskCount
}

func (store *RuntimeSettingsStore) Refresh(ctx context.Context) error {
	if store == nil || store.repository == nil {
		return errors.New("runtime settings repository is required")
	}
	store.refreshMu.Lock()
	defer store.refreshMu.Unlock()
	rows, err := store.repository.List(ctx, settingKeys())
	if err != nil {
		return err
	}
	indexed, err := validateSettingRows(rows)
	if err != nil {
		return err
	}
	version := runtimeSettingsVersion(indexed)
	if version <= store.version.Load() {
		return nil
	}
	values := make(map[string]string, len(indexed))
	for key, row := range indexed {
		values[key] = row.Value
	}
	snapshot, err := store.Build(values)
	if err != nil {
		return err
	}
	return store.StoreVersioned(snapshot, version)
}

func (store *RuntimeSettingsStore) Start(parent context.Context) error {
	if store == nil || parent == nil || store.repository == nil || store.pollInterval <= 0 {
		return errors.New("runtime settings refresh dependencies are required")
	}
	store.lifecycleMu.Lock()
	defer store.lifecycleMu.Unlock()
	if store.cancel != nil {
		return errors.New("runtime settings refresh is already started")
	}
	ctx, cancel := context.WithCancel(parent)
	store.cancel = cancel
	store.done = make(chan struct{})
	store.ready.Store(true)
	go store.run(ctx, store.done)
	return nil
}

func (store *RuntimeSettingsStore) run(ctx context.Context, done chan<- struct{}) {
	defer close(done)
	ticker := time.NewTicker(store.pollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := store.Refresh(ctx); err != nil && ctx.Err() == nil {
				log.Printf("runtime settings refresh failed: %v", err)
			}
		}
	}
}

func (store *RuntimeSettingsStore) Quiesce() error {
	if store != nil {
		store.ready.Store(false)
	}
	return nil
}

func (store *RuntimeSettingsStore) Stop(ctx context.Context) error {
	if store == nil {
		return nil
	}
	store.lifecycleMu.Lock()
	cancel, done := store.cancel, store.done
	store.cancel = nil
	store.done = nil
	store.lifecycleMu.Unlock()
	store.ready.Store(false)
	if cancel == nil {
		return nil
	}
	cancel()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (store *RuntimeSettingsStore) Ready() bool {
	return store != nil && store.ready.Load()
}

func runtimeSettingsVersion(rows map[string]model.PlatformSetting) int64 {
	var version int64
	for _, row := range rows {
		version += row.UpdatedAt
	}
	return version
}

func runtimeSettingsFromValues(values map[string]string, clock common.Clock, governors ...UpstreamGovernor) (RuntimeSettingsSnapshot, error) {
	integer := func(key string) (int, error) {
		value, err := strconv.Atoi(values[key])
		if err != nil || value <= 0 {
			return 0, errors.New("runtime setting is invalid: " + key)
		}
		return value, nil
	}
	retention, err := integer("fast_task.history_retention_seconds")
	if err != nil {
		return RuntimeSettingsSnapshot{}, err
	}
	count, err := integer("fast_task.history_count")
	if err != nil {
		return RuntimeSettingsSnapshot{}, err
	}
	hosts, err := parseCanonicalUpstreamHostSuffixes(values["upstream.allowed_host_suffixes"])
	if err != nil {
		return RuntimeSettingsSnapshot{}, err
	}
	cidrs, err := parseCanonicalUpstreamCIDRs(values["upstream.allowed_cidrs"])
	if err != nil {
		return RuntimeSettingsSnapshot{}, err
	}
	connect, err := integer("upstream.connect_timeout_seconds")
	if err != nil {
		return RuntimeSettingsSnapshot{}, err
	}
	header, err := integer("upstream.response_header_timeout_seconds")
	if err != nil {
		return RuntimeSettingsSnapshot{}, err
	}
	request, err := integer("upstream.request_timeout_seconds")
	if err != nil {
		return RuntimeSettingsSnapshot{}, err
	}
	export, err := integer("upstream.export_timeout_seconds")
	if err != nil {
		return RuntimeSettingsSnapshot{}, err
	}
	if connect > request || header > request || request > export {
		return RuntimeSettingsSnapshot{}, errors.New("upstream timeout relationships are invalid")
	}
	requests, err := integer("upstream.rate_limit_requests")
	if err != nil {
		return RuntimeSettingsSnapshot{}, err
	}
	window, err := integer("upstream.rate_limit_window_seconds")
	if err != nil {
		return RuntimeSettingsSnapshot{}, err
	}
	maxInFlight, err := integer("upstream.max_inflight_per_origin")
	if err != nil {
		return RuntimeSettingsSnapshot{}, err
	}
	governorOptions := UpstreamGovernorOptions{
		Requests: requests, Window: time.Duration(window) * time.Second,
		MaxInFlight: maxInFlight, Clock: clock,
	}
	var governor UpstreamGovernor
	if len(governors) > 0 {
		governor = governors[0]
	}
	if governor == nil {
		var err error
		governor, err = NewUpstreamGovernor(governorOptions)
		if err != nil {
			return RuntimeSettingsSnapshot{}, err
		}
	}
	return RuntimeSettingsSnapshot{
		FastTaskRetention: time.Duration(retention) * time.Second, FastTaskCount: count,
		AllowedHosts: hosts, AllowedCIDRs: cidrs,
		ConnectTimeout:  time.Duration(connect) * time.Second,
		HeaderTimeout:   time.Duration(header) * time.Second,
		RequestTimeout:  time.Duration(request) * time.Second,
		ExportTimeout:   time.Duration(export) * time.Second,
		Governor:        governor,
		governorOptions: governorOptions,
	}, nil
}

func canonicalUpstreamHostSuffixes(raw string) (string, error) {
	values := splitRuntimeList(raw)
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimPrefix(strings.TrimSuffix(strings.ToLower(value), "."), "*.")
		ascii, err := idna.Lookup.ToASCII(value)
		if err != nil || ascii == "" || strings.ContainsAny(ascii, "/:@") {
			return "", errors.New("invalid upstream host suffix")
		}
		if _, err := netip.ParseAddr(ascii); err == nil {
			return "", errors.New("upstream host suffix must be a DNS name")
		}
		if _, exists := seen[ascii]; !exists {
			seen[ascii] = struct{}{}
			result = append(result, ascii)
		}
	}
	sort.Strings(result)
	return strings.Join(result, ","), nil
}

func canonicalUpstreamCIDRs(raw string) (string, error) {
	values := splitRuntimeList(raw)
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		prefix, err := netip.ParsePrefix(value)
		if err != nil {
			address, addressErr := netip.ParseAddr(value)
			if addressErr != nil {
				return "", errors.New("invalid upstream CIDR")
			}
			prefix = netip.PrefixFrom(address, address.BitLen())
		}
		canonical := prefix.Masked().String()
		if _, exists := seen[canonical]; !exists {
			seen[canonical] = struct{}{}
			result = append(result, canonical)
		}
	}
	sort.Strings(result)
	return strings.Join(result, ","), nil
}

func parseCanonicalUpstreamHostSuffixes(raw string) ([]string, error) {
	canonical, err := canonicalUpstreamHostSuffixes(raw)
	if err != nil || canonical != raw {
		return nil, errors.New("stored upstream host suffixes are invalid")
	}
	return splitRuntimeList(canonical), nil
}

func parseCanonicalUpstreamCIDRs(raw string) ([]netip.Prefix, error) {
	canonical, err := canonicalUpstreamCIDRs(raw)
	if err != nil || canonical != raw {
		return nil, errors.New("stored upstream CIDRs are invalid")
	}
	values := splitRuntimeList(canonical)
	result := make([]netip.Prefix, 0, len(values))
	for _, value := range values {
		prefix, parseErr := netip.ParsePrefix(value)
		if parseErr != nil {
			return nil, parseErr
		}
		result = append(result, prefix)
	}
	return result, nil
}

func splitRuntimeList(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.FieldsFunc(raw, func(character rune) bool {
		return character == ',' || character == '\n' || character == '\r'
	})
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if value := strings.TrimSpace(part); value != "" {
			result = append(result, value)
		}
	}
	return result
}
