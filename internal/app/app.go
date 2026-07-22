package app

import (
	"context"
	"errors"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/Willxup/flowlens/internal/attribution"
	"github.com/Willxup/flowlens/internal/backup"
	"github.com/Willxup/flowlens/internal/buildinfo"
	"github.com/Willxup/flowlens/internal/clashapi"
	"github.com/Willxup/flowlens/internal/collector"
	"github.com/Willxup/flowlens/internal/config"
	"github.com/Willxup/flowlens/internal/httpapi"
	"github.com/Willxup/flowlens/internal/maintenance"
	"github.com/Willxup/flowlens/internal/query"
	flowstatus "github.com/Willxup/flowlens/internal/status"
	"github.com/Willxup/flowlens/internal/storage"
	"github.com/Willxup/flowlens/internal/webassets"
)

// App owns the Stage 1 process dependencies.
type App struct {
	listen      string
	store       *storage.Store
	runtime     *Runtime
	handler     http.Handler
	status      *flowstatus.Tracker
	maintenance *maintenance.Runner
	attribution *attribution.Tracker

	closeOnce sync.Once
	closeErr  error
}

// New performs the ordered Stage 1 startup checks.
func New(ctx context.Context, cfg config.Config) (*App, error) {
	store, err := storage.Open(ctx, storage.Options{
		DatabasePath: cfg.Storage.DatabasePath, SoftLimitBytes: int64(cfg.Storage.SoftLimit),
	})
	if err != nil {
		return nil, err
	}
	fail := func(err error) (*App, error) {
		_ = store.Close()
		return nil, err
	}
	if err := os.MkdirAll(cfg.Backup.Directory, 0o700); err != nil {
		return fail(errors.New("cannot create FlowLens backup directory"))
	}
	status, err := store.InspectSchema(ctx)
	if err != nil {
		return fail(err)
	}
	preUpgradePath := ""
	if status.NeedsMigration {
		if status.CurrentVersion > 0 {
			preUpgradePath, err = backup.CreatePreUpgrade(ctx, store, cfg.Backup.Directory, time.Now())
			if err != nil {
				return fail(errors.New("cannot create FlowLens pre-upgrade backup"))
			}
		}
		if _, err := store.Migrate(ctx); err != nil {
			return fail(err)
		}
	}
	if err := store.QuickCheck(ctx); err != nil {
		return fail(err)
	}
	if _, err := store.InspectSchema(ctx); err != nil {
		return fail(err)
	}
	if preUpgradePath != "" {
		if err := backup.RemovePreUpgrade(preUpgradePath); err != nil {
			return fail(errors.New("cannot remove FlowLens pre-upgrade backup"))
		}
	}
	client, err := clashapi.New(clashapi.Options{
		BaseURL: cfg.ClashAPI.URL, Secret: cfg.ClashAPI.Secret.Value(),
		RequestTimeout: cfg.ClashAPI.RequestTimeout.Duration, MaxResponseSize: int64(cfg.ClashAPI.MaxResponseSize),
	})
	if err != nil {
		return fail(errors.New("cannot configure FlowLens Clash API client"))
	}
	capabilities, err := client.Probe(ctx)
	if err != nil {
		return fail(errors.New("required FlowLens Clash API capability is unavailable"))
	}
	attributionTracker, err := attribution.NewTracker(attribution.Options{
		TopK:         cfg.Retention.TopK,
		SourceMode:   attribution.SourceMode(cfg.Privacy.SourceMode),
		IPv4Prefix:   cfg.Privacy.SourceIPv4Prefix,
		IPv6Prefix:   cfg.Privacy.SourceIPv6Prefix,
		Capabilities: capabilities.Dimensions,
	})
	if err != nil {
		return fail(errors.New("cannot configure FlowLens attribution"))
	}
	tracker := flowstatus.NewTracker()
	ring, err := collector.NewRing(collector.DefaultRingCapacity)
	if err != nil {
		return fail(errors.New("cannot create FlowLens live ring"))
	}
	runtime, err := NewRuntime(ctx, RuntimeOptions{
		Client: client, Store: store, Ring: ring, Status: tracker,
		BucketTimezone: cfg.Time.Timezone, ConnectionsInterval: cfg.ClashAPI.ConnectionsInterval.Duration,
		Attribution: attributionTracker, TopK: cfg.Retention.TopK,
	})
	if err != nil {
		return fail(err)
	}
	location, err := time.LoadLocation(cfg.Time.Timezone)
	if err != nil {
		return fail(errors.New("cannot load FlowLens bucket timezone"))
	}
	queries, err := query.NewService(query.Options{
		Store: store, Live: attributionTracker, Now: time.Now, Retention: cfg.Retention,
		Location: location, PrivacyMode: attribution.SourceMode(cfg.Privacy.SourceMode),
	})
	if err != nil {
		return fail(errors.New("cannot configure FlowLens historical queries"))
	}
	web, err := webassets.Content()
	if err != nil {
		return fail(errors.New("cannot load FlowLens web assets"))
	}
	handler, err := httpapi.New(httpapi.Options{
		AccessKey: cfg.Auth.AccessKey.Value(), SessionTTL: cfg.Auth.SessionTTL.Duration,
		Status: tracker, Queries: queries, CapabilitySource: attributionTracker, Timezone: cfg.Time.Timezone,
		Web: web, Live: ring,
	})
	if err != nil {
		return fail(err)
	}
	maintenanceRunner, err := maintenance.New(maintenance.Options{
		Store: store, Location: location, Retention: cfg.Retention,
		Backup: backup.Options{
			Store: store, Directory: cfg.Backup.Directory,
			DailyKeep: cfg.Backup.DailyKeep, MonthlyKeep: cfg.Backup.MonthlyKeep,
			BucketTimezone: cfg.Time.Timezone, ApplicationVersion: buildinfo.Version,
		},
		BackupTime: cfg.Backup.LocalTime,
	})
	if err != nil {
		return fail(errors.New("cannot configure FlowLens maintenance"))
	}
	_ = tracker.Set(flowstatus.LevelOK, "ready", true)
	return &App{
		listen: cfg.Server.Listen, store: store, runtime: runtime, handler: handler,
		status: tracker, maintenance: maintenanceRunner, attribution: attributionTracker,
	}, nil
}

// Handler returns the complete minimal HTTP boundary.
func (a *App) Handler() http.Handler { return a.handler }

// Run serves HTTP and collection until cancellation or a fatal error.
func (a *App) Run(ctx context.Context) error {
	runContext, cancel := context.WithCancel(ctx)
	defer cancel()
	server := &http.Server{Addr: a.listen, Handler: a.handler, ReadHeaderTimeout: 5_000_000_000}
	errorsChannel := make(chan error, 2)
	var components sync.WaitGroup
	components.Add(3)
	go func() {
		defer components.Done()
		errorsChannel <- a.runtime.Run(runContext)
	}()
	go func() {
		defer components.Done()
		a.runMaintenance(runContext)
	}()
	go func() {
		defer components.Done()
		err := server.ListenAndServe()
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
		errorsChannel <- err
	}()
	var result error
	select {
	case <-ctx.Done():
	case err := <-errorsChannel:
		if err != nil {
			result = errors.New("FlowLens runtime stopped")
		}
	}
	cancel()
	shutdownContext, shutdownCancel := context.WithTimeout(context.Background(), 5_000_000_000)
	if err := server.Shutdown(shutdownContext); err != nil {
		_ = server.Close()
	}
	shutdownCancel()
	components.Wait()
	return result
}

// Close releases the store and its data-directory lock exactly once.
func (a *App) Close() error {
	a.closeOnce.Do(func() { a.closeErr = a.store.Close() })
	return a.closeErr
}
