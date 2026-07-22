package cli

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/Willxup/flowlens/internal/clashapi"
	"github.com/Willxup/flowlens/internal/config"
	"github.com/Willxup/flowlens/internal/storage"
)

func runDoctor(
	ctx context.Context,
	stdout io.Writer,
	stderr io.Writer,
	dependencies Dependencies,
) int {
	loadConfig := dependencies.LoadConfig
	if loadConfig == nil {
		loadConfig = config.Load
	}
	cfg, err := loadConfig()
	if err != nil {
		fmt.Fprintln(stderr, "FlowLens doctor configuration check failed")
		return 1
	}
	fmt.Fprintln(stdout, "config: ok")

	info, err := os.Lstat(cfg.Storage.DatabasePath)
	if err != nil || !info.Mode().IsRegular() {
		fmt.Fprintln(stderr, "FlowLens doctor storage check failed")
		return 1
	}
	store, err := storage.Open(ctx, storage.Options{
		DatabasePath: cfg.Storage.DatabasePath, SoftLimitBytes: int64(cfg.Storage.SoftLimit),
	})
	if err != nil {
		fmt.Fprintln(stderr, "FlowLens doctor storage check failed")
		return 1
	}
	storageOK := false
	if err := store.QuickCheck(ctx); err == nil {
		status, inspectErr := store.InspectSchema(ctx)
		storageOK = inspectErr == nil && status.CurrentVersion > 0 &&
			status.CurrentVersion == status.LatestVersion && !status.NeedsMigration
	}
	closeErr := store.Close()
	if !storageOK || closeErr != nil {
		fmt.Fprintln(stderr, "FlowLens doctor storage check failed")
		return 1
	}
	fmt.Fprintln(stdout, "storage: ok")

	client, err := clashapi.New(clashapi.Options{
		BaseURL: cfg.ClashAPI.URL, Secret: cfg.ClashAPI.Secret.Value(),
		RequestTimeout:  cfg.ClashAPI.RequestTimeout.Duration,
		MaxResponseSize: int64(cfg.ClashAPI.MaxResponseSize), HTTPClient: dependencies.HTTPClient,
	})
	if err != nil {
		fmt.Fprintln(stderr, "FlowLens doctor Clash API check failed")
		return 1
	}
	if _, err := client.Probe(ctx); err != nil {
		fmt.Fprintln(stderr, "FlowLens doctor Clash API check failed")
		return 1
	}
	fmt.Fprintln(stdout, "clash_api: ok")
	return 0
}
