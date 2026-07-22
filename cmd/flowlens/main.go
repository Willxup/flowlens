package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/Willxup/flowlens/internal/cli"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := cli.Run(ctx, os.Args[1:], os.Stdout, os.Stderr, cli.DefaultDependencies())
	stop()
	os.Exit(code)
}
