package cli_test

import (
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Willxup/flowlens/internal/cli"
	"github.com/Willxup/flowlens/internal/config"
)

func TestHealthcheckUsesLoopbackAndRequiresNoContent(t *testing.T) {
	requestedPath := ""
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requestedPath = request.URL.Path
		writer.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	_, port, err := net.SplitHostPort(strings.TrimPrefix(server.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	dependencies := cli.Dependencies{
		LoadConfig: func() (config.Config, error) {
			return config.Config{Server: config.Server{Listen: "0.0.0.0:" + port}}, nil
		},
		HTTPClient: server.Client(),
	}

	code, stdout, stderr := runWithDependencies([]string{"healthcheck"}, dependencies)
	if code != 0 || stdout != "healthcheck: ok\n" || stderr != "" {
		t.Fatalf("healthcheck = (%d, %q, %q)", code, stdout, stderr)
	}
	if requestedPath != "/healthz" {
		t.Fatalf("requested path = %q", requestedPath)
	}
}

func TestHealthcheckRejectsNonNoContentResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	_, port, err := net.SplitHostPort(strings.TrimPrefix(server.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	dependencies := cli.Dependencies{
		LoadConfig: func() (config.Config, error) {
			return config.Config{Server: config.Server{Listen: "127.0.0.1:" + port}}, nil
		},
		HTTPClient: server.Client(),
	}

	code, stdout, stderr := runWithDependencies([]string{"healthcheck"}, dependencies)
	if code != 1 || stdout != "" || stderr != "FlowLens healthcheck failed\n" {
		t.Fatalf("healthcheck failure = (%d, %q, %q)", code, stdout, stderr)
	}
}
