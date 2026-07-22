package cli_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/Willxup/flowlens/internal/buildinfo"
	"github.com/Willxup/flowlens/internal/cli"
)

func TestVersionIsStableAndDoesNotReadConfig(t *testing.T) {
	previous := buildinfo.Version
	buildinfo.Version = "dev"
	t.Cleanup(func() { buildinfo.Version = previous })

	code, stdout, stderr := run([]string{"version"})
	if code != 0 || stdout != "FlowLens dev\n" || stderr != "" {
		t.Fatalf("version = (%d, %q, %q)", code, stdout, stderr)
	}
}

func TestUnknownCommandPrintsOnlyUsage(t *testing.T) {
	code, stdout, stderr := run([]string{"unknown"})
	const usage = "usage: flowlens <serve|version|healthcheck|doctor|backup|restore>\n"
	if code != 2 || stdout != "" || stderr != usage {
		t.Fatalf("unknown command = (%d, %q, %q)", code, stdout, stderr)
	}
}

func run(args []string) (int, string, string) {
	return runWithDependencies(args, cli.Dependencies{})
}

func runWithDependencies(args []string, dependencies cli.Dependencies) (int, string, string) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := cli.Run(context.Background(), args, &stdout, &stderr, dependencies)
	return code, stdout.String(), stderr.String()
}
