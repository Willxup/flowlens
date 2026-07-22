package cli

import (
	"context"
	"fmt"
	"io"

	"github.com/Willxup/flowlens/internal/backup"
	"github.com/Willxup/flowlens/internal/config"
	"github.com/Willxup/flowlens/internal/migrations"
)

func runRestore(
	ctx context.Context,
	args []string,
	stdout io.Writer,
	stderr io.Writer,
	dependencies Dependencies,
) int {
	checkOnly := len(args) == 3 && args[1] == "--check"
	writeOutput := len(args) == 4 && args[1] == "--output"
	if !checkOnly && !writeOutput {
		fmt.Fprintln(stderr, usage)
		return 2
	}
	fail := func() int {
		fmt.Fprintln(stderr, "FlowLens restore failed")
		return 1
	}
	loadConfig := dependencies.LoadConfig
	if loadConfig == nil {
		loadConfig = config.Load
	}
	cfg, err := loadConfig()
	if err != nil {
		return fail()
	}
	latest, err := migrations.LatestVersion()
	if err != nil {
		return fail()
	}
	policy := backup.ValidationPolicy{
		ExpectedBucketTimezone: cfg.Time.Timezone,
		MaximumSchemaVersion:   latest,
	}
	if checkOnly {
		if _, err := backup.Validate(ctx, args[2], policy); err != nil {
			return fail()
		}
		fmt.Fprintln(stdout, "restore check: ok")
		return 0
	}
	outputPath := args[2]
	if outputPath == cfg.Storage.DatabasePath {
		return fail()
	}
	if _, err := backup.Restore(ctx, args[3], outputPath, policy); err != nil {
		return fail()
	}
	fmt.Fprintln(stdout, "restore: ok")
	return 0
}
