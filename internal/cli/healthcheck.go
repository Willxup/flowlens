package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/Willxup/flowlens/internal/config"
)

func runHealthcheck(
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
		fmt.Fprintln(stderr, "FlowLens healthcheck failed")
		return 1
	}
	url, err := healthURL(cfg.Server.Listen)
	if err != nil {
		fmt.Fprintln(stderr, "FlowLens healthcheck failed")
		return 1
	}
	client := dependencies.HTTPClient
	if client == nil {
		transport, ok := http.DefaultTransport.(*http.Transport)
		if !ok {
			fmt.Fprintln(stderr, "FlowLens healthcheck failed")
			return 1
		}
		localTransport := transport.Clone()
		localTransport.Proxy = nil
		client = &http.Client{Transport: localTransport, Timeout: 3 * time.Second}
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		fmt.Fprintln(stderr, "FlowLens healthcheck failed")
		return 1
	}
	response, err := client.Do(request)
	if err != nil {
		fmt.Fprintln(stderr, "FlowLens healthcheck failed")
		return 1
	}
	closeErr := response.Body.Close()
	if response.StatusCode != http.StatusNoContent || closeErr != nil {
		fmt.Fprintln(stderr, "FlowLens healthcheck failed")
		return 1
	}
	fmt.Fprintln(stdout, "healthcheck: ok")
	return 0
}

func healthURL(listen string) (string, error) {
	_, port, err := net.SplitHostPort(listen)
	if err != nil || port == "" {
		return "", errors.New("invalid FlowLens health address")
	}
	return "http://127.0.0.1:" + port + "/healthz", nil
}
