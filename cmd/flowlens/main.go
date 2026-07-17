package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/Willxup/flowlens/internal/app"
	"github.com/Willxup/flowlens/internal/config"
)

func main() {
	if len(os.Args) != 2 || os.Args[1] != "serve" {
		fmt.Fprintln(os.Stderr, "usage: flowlens serve")
		os.Exit(2)
	}
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "FlowLens configuration is unavailable")
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	application, err := app.New(ctx, cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, "FlowLens startup failed")
		os.Exit(1)
	}
	defer application.Close()
	if err := application.Run(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "FlowLens runtime failed")
		os.Exit(1)
	}
}
