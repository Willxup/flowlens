// Package cli implements the FlowLens command-line boundary.
package cli

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/Willxup/flowlens/internal/app"
	"github.com/Willxup/flowlens/internal/buildinfo"
	"github.com/Willxup/flowlens/internal/config"
)

const usage = "usage: flowlens <serve|version|healthcheck|doctor|backup|restore>"

// Application is the runtime boundary owned by the serve command.
type Application interface {
	Run(context.Context) error
	Close() error
}

// Dependencies contains production operations replaced by command tests.
type Dependencies struct {
	LoadConfig     func() (config.Config, error)
	NewApplication func(context.Context, config.Config) (Application, error)
	HTTPClient     *http.Client
	Now            func() time.Time
}

// DefaultDependencies returns the real FlowLens command dependencies.
func DefaultDependencies() Dependencies {
	return Dependencies{
		LoadConfig: config.Load,
		NewApplication: func(ctx context.Context, cfg config.Config) (Application, error) {
			return app.New(ctx, cfg)
		},
		Now: time.Now,
	}
}

// Run dispatches one FlowLens command and returns its process exit code.
func Run(
	ctx context.Context,
	args []string,
	stdout io.Writer,
	stderr io.Writer,
	dependencies Dependencies,
) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, usage)
		return 2
	}
	switch args[0] {
	case "version":
		if len(args) != 1 {
			fmt.Fprintln(stderr, usage)
			return 2
		}
		fmt.Fprintf(stdout, "FlowLens %s\n", buildinfo.Version)
		return 0
	case "serve":
		if len(args) != 1 {
			fmt.Fprintln(stderr, usage)
			return 2
		}
		return runServe(ctx, stderr, dependencies)
	case "healthcheck":
		if len(args) != 1 {
			fmt.Fprintln(stderr, usage)
			return 2
		}
		return runHealthcheck(ctx, stdout, stderr, dependencies)
	case "doctor":
		if len(args) != 1 {
			fmt.Fprintln(stderr, usage)
			return 2
		}
		return runDoctor(ctx, stdout, stderr, dependencies)
	case "backup":
		if len(args) != 1 {
			fmt.Fprintln(stderr, usage)
			return 2
		}
		return runBackup(ctx, stdout, stderr, dependencies)
	case "restore":
		return runRestore(ctx, args, stdout, stderr, dependencies)
	default:
		fmt.Fprintln(stderr, usage)
		return 2
	}
}

func runServe(ctx context.Context, stderr io.Writer, dependencies Dependencies) int {
	if dependencies.LoadConfig == nil {
		dependencies.LoadConfig = config.Load
	}
	if dependencies.NewApplication == nil {
		dependencies.NewApplication = DefaultDependencies().NewApplication
	}
	cfg, err := dependencies.LoadConfig()
	if err != nil {
		fmt.Fprintln(stderr, "FlowLens configuration is unavailable")
		return 1
	}
	application, err := dependencies.NewApplication(ctx, cfg)
	if err != nil {
		fmt.Fprintln(stderr, "FlowLens startup failed")
		return 1
	}
	defer application.Close()
	if err := application.Run(ctx); err != nil {
		fmt.Fprintln(stderr, "FlowLens runtime failed")
		return 1
	}
	return 0
}
