package app

import (
	"context"
	"errors"
	"net/http"
	"sync"

	"github.com/Willxup/flowlens/internal/clashapi"
	"github.com/Willxup/flowlens/internal/collector"
	"github.com/Willxup/flowlens/internal/config"
	"github.com/Willxup/flowlens/internal/httpapi"
	flowstatus "github.com/Willxup/flowlens/internal/status"
	"github.com/Willxup/flowlens/internal/storage"
)

// App owns the Stage 1 process dependencies.
type App struct {
	listen  string
	store   *storage.Store
	runtime *Runtime
	handler http.Handler

	closeOnce sync.Once
	closeErr  error
}

// New performs the ordered Stage 1 startup checks.
func New(ctx context.Context, cfg config.Config) (*App, error) {
	store, err := storage.Open(ctx, storage.Options{DatabasePath: cfg.Storage.DatabasePath})
	if err != nil {
		return nil, err
	}
	fail := func(err error) (*App, error) {
		_ = store.Close()
		return nil, err
	}
	status, err := store.InspectSchema(ctx)
	if err != nil {
		return fail(err)
	}
	if status.NeedsMigration {
		if _, err := store.Migrate(ctx); err != nil {
			return fail(err)
		}
	}
	if err := store.QuickCheck(ctx); err != nil {
		return fail(err)
	}
	client, err := clashapi.New(clashapi.Options{
		BaseURL: cfg.ClashAPI.URL, Secret: cfg.ClashAPI.Secret.Value(),
		RequestTimeout: cfg.ClashAPI.RequestTimeout.Duration, MaxResponseSize: int64(cfg.ClashAPI.MaxResponseSize),
	})
	if err != nil {
		return fail(errors.New("cannot configure FlowLens Clash API client"))
	}
	if _, err := client.Probe(ctx); err != nil {
		return fail(errors.New("required FlowLens Clash API capability is unavailable"))
	}
	tracker := flowstatus.NewTracker()
	ring, err := collector.NewRing(collector.DefaultRingCapacity)
	if err != nil {
		return fail(errors.New("cannot create FlowLens live ring"))
	}
	runtime, err := NewRuntime(ctx, RuntimeOptions{
		Client: client, Store: store, Ring: ring, Status: tracker,
		BucketTimezone: cfg.Time.Timezone, ConnectionsInterval: cfg.ClashAPI.ConnectionsInterval.Duration,
	})
	if err != nil {
		return fail(err)
	}
	handler, err := httpapi.New(httpapi.Options{
		AccessKey: cfg.Auth.AccessKey.Value(), SessionTTL: cfg.Auth.SessionTTL.Duration, Status: tracker,
	})
	if err != nil {
		return fail(err)
	}
	_ = tracker.Set(flowstatus.LevelOK, "ready", true)
	return &App{listen: cfg.Server.Listen, store: store, runtime: runtime, handler: handler}, nil
}

// Handler returns the complete minimal HTTP boundary.
func (a *App) Handler() http.Handler { return a.handler }

// Run serves HTTP and collection until cancellation or a fatal error.
func (a *App) Run(ctx context.Context) error {
	server := &http.Server{Addr: a.listen, Handler: a.handler, ReadHeaderTimeout: 5_000_000_000}
	errorsChannel := make(chan error, 2)
	go func() { errorsChannel <- a.runtime.Run(ctx) }()
	go func() {
		err := server.ListenAndServe()
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
		errorsChannel <- err
	}()
	select {
	case <-ctx.Done():
		shutdownContext, cancel := context.WithTimeout(context.Background(), 5_000_000_000)
		defer cancel()
		_ = server.Shutdown(shutdownContext)
		return nil
	case err := <-errorsChannel:
		_ = server.Close()
		if err != nil {
			return errors.New("FlowLens runtime stopped")
		}
		return nil
	}
}

// Close releases the store and its data-directory lock exactly once.
func (a *App) Close() error {
	a.closeOnce.Do(func() { a.closeErr = a.store.Close() })
	return a.closeErr
}
