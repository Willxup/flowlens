package clashapi_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Willxup/flowlens/internal/clashapi"
)

func TestProbeDetectsRequiredOptionalAndDimensionCapabilities(t *testing.T) {
	scenario := validProbeScenario(t)
	client, closeServer := probeClient(t, scenario)
	defer closeServer()

	capabilities, err := client.Probe(context.Background())
	if err != nil {
		t.Fatalf("Probe() error = %v", err)
	}
	if !capabilities.Version || !capabilities.Traffic || !capabilities.Connections || !capabilities.Memory {
		t.Errorf("required/optional capabilities = %#v", capabilities)
	}
	if capabilities.VersionValue != "sing-box 1.12.0-fixture" {
		t.Errorf("VersionValue = %q", capabilities.VersionValue)
	}
	wantDimensions := clashapi.DimensionCapabilities{
		ConnectionID:    true,
		SourceIP:        true,
		DestinationIP:   true,
		DestinationPort: true,
		Network:         true,
		Host:            true,
	}
	if capabilities.Dimensions != wantDimensions {
		t.Errorf("Dimensions = %#v, want %#v", capabilities.Dimensions, wantDimensions)
	}
	if scenario.connectionCalls() != 2 {
		t.Errorf("/connections calls = %d", scenario.connectionCalls())
	}
}

func TestProbeTreatsMemoryAsOptional(t *testing.T) {
	scenario := validProbeScenario(t)
	scenario.memoryStatus = http.StatusNotFound
	scenario.memoryBody = fixtureResponseSentinel
	client, closeServer := probeClient(t, scenario)
	defer closeServer()

	capabilities, err := client.Probe(context.Background())
	if err != nil {
		t.Fatalf("Probe() error = %v", err)
	}
	if capabilities.Memory {
		t.Error("Memory = true")
	}
	if !capabilities.Version || !capabilities.Traffic || !capabilities.Connections {
		t.Errorf("required capabilities = %#v", capabilities)
	}
}

func TestProbeRejectsEmptyOrMalformedVersion(t *testing.T) {
	for name, body := range map[string]string{
		"empty":     `{"version":"   "}`,
		"malformed": `{"version":`,
	} {
		t.Run(name, func(t *testing.T) {
			scenario := validProbeScenario(t)
			scenario.versionBody = body
			client, closeServer := probeClient(t, scenario)
			defer closeServer()

			_, err := client.Probe(context.Background())
			assertProbeEndpointError(t, err, "/version")
		})
	}
}

func TestProbeRejectsNegativeRequiredSamples(t *testing.T) {
	for name, mutate := range map[string]func(*probeScenario){
		"connections": func(s *probeScenario) {
			s.connectionBodies = []string{`{"uploadTotal":-1,"downloadTotal":0,"connections":[]}`}
		},
		"traffic": func(s *probeScenario) {
			s.trafficBody = `{"up":-1,"down":0}` + "\n"
		},
	} {
		t.Run(name, func(t *testing.T) {
			scenario := validProbeScenario(t)
			mutate(scenario)
			client, closeServer := probeClient(t, scenario)
			defer closeServer()

			_, err := client.Probe(context.Background())
			assertProbeEndpointError(t, err, "/"+name)
		})
	}
}

func TestProbeRejectsMissingRequiredEndpoint(t *testing.T) {
	for _, endpoint := range []string{"/version", "/connections", "/traffic"} {
		t.Run(endpoint, func(t *testing.T) {
			scenario := validProbeScenario(t)
			switch endpoint {
			case "/version":
				scenario.versionStatus = http.StatusNotFound
				scenario.versionBody = fixtureResponseSentinel
			case "/connections":
				scenario.connectionsStatus = http.StatusNotFound
				scenario.connectionBodies = []string{fixtureResponseSentinel}
			case "/traffic":
				scenario.trafficStatus = http.StatusNotFound
				scenario.trafficBody = fixtureResponseSentinel
			}
			client, closeServer := probeClient(t, scenario)
			defer closeServer()

			_, err := client.Probe(context.Background())
			assertProbeEndpointError(t, err, endpoint)
		})
	}
}

func TestProbeDerivesDimensionsAcrossBothSnapshots(t *testing.T) {
	scenario := validProbeScenario(t)
	scenario.connectionBodies = []string{
		`{"uploadTotal":1,"downloadTotal":2,"connections":[{"id":"fixture-id","metadata":{"sourceIP":"192.0.2.10"},"upload":0,"download":0}]}`,
		`{"uploadTotal":2,"downloadTotal":3,"connections":[{"metadata":{"destinationIP":"198.51.100.20","destinationPort":"443","network":"tcp","host":"api.example.test"},"upload":0,"download":0}]}`,
	}
	client, closeServer := probeClient(t, scenario)
	defer closeServer()

	capabilities, err := client.Probe(context.Background())
	if err != nil {
		t.Fatalf("Probe() error = %v", err)
	}
	want := clashapi.DimensionCapabilities{
		ConnectionID:    true,
		SourceIP:        true,
		DestinationIP:   true,
		DestinationPort: true,
		Network:         true,
		Host:            true,
	}
	if capabilities.Dimensions != want {
		t.Errorf("Dimensions = %#v, want %#v", capabilities.Dimensions, want)
	}
}

func TestProbeDisablesUnobservedDimensions(t *testing.T) {
	scenario := validProbeScenario(t)
	scenario.connectionBodies = []string{
		`{"uploadTotal":1,"downloadTotal":2,"connections":[{"metadata":{},"upload":0,"download":0}]}`,
		`{"uploadTotal":2,"downloadTotal":3,"connections":[]}`,
	}
	client, closeServer := probeClient(t, scenario)
	defer closeServer()

	capabilities, err := client.Probe(context.Background())
	if err != nil {
		t.Fatalf("Probe() error = %v", err)
	}
	if capabilities.Dimensions != (clashapi.DimensionCapabilities{}) {
		t.Errorf("Dimensions = %#v", capabilities.Dimensions)
	}
}

func TestProbeUsesAnIndependentTimeoutForEachEndpoint(t *testing.T) {
	scenario := validProbeScenario(t)
	scenario.responseDelay = 20 * time.Millisecond
	client, closeServer := probeClientWithTimeout(t, scenario, 50*time.Millisecond)
	defer closeServer()

	started := time.Now()
	capabilities, err := client.Probe(context.Background())
	if err != nil {
		t.Fatalf("Probe() error = %v", err)
	}
	if !capabilities.Memory {
		t.Error("Memory = false")
	}
	if elapsed := time.Since(started); elapsed < 80*time.Millisecond {
		t.Fatalf("Probe() elapsed = %v; test did not span a single-operation timeout", elapsed)
	}
}

func assertProbeEndpointError(t *testing.T, err error, endpoint string) {
	t.Helper()
	if err == nil {
		t.Fatal("Probe() error = nil")
	}
	want := "Clash API probe failed at " + endpoint
	if err.Error() != want {
		t.Errorf("Probe() error = %q, want %q", err, want)
	}
	assertNoProbeLeak(t, err)
}

type probeScenario struct {
	versionStatus     int
	versionBody       string
	connectionsStatus int
	connectionBodies  []string
	trafficStatus     int
	trafficBody       string
	memoryStatus      int
	memoryBody        string
	responseDelay     time.Duration

	mu                  sync.Mutex
	connectionsRequests int
}

func validProbeScenario(t *testing.T) *probeScenario {
	t.Helper()
	return &probeScenario{
		versionStatus:     http.StatusOK,
		versionBody:       string(fixture(t, "version.json")),
		connectionsStatus: http.StatusOK,
		connectionBodies: []string{
			string(fixture(t, "connections-1.json")),
			string(fixture(t, "connections-2.json")),
		},
		trafficStatus: http.StatusOK,
		trafficBody:   string(fixture(t, "traffic.ndjson")),
		memoryStatus:  http.StatusOK,
		memoryBody:    string(fixture(t, "memory.ndjson")),
	}
}

func (s *probeScenario) connectionCalls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.connectionsRequests
}

func (s *probeScenario) nextConnectionBody() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	index := s.connectionsRequests
	s.connectionsRequests++
	if index >= len(s.connectionBodies) {
		index = len(s.connectionBodies) - 1
	}
	return s.connectionBodies[index]
}

func probeClient(t *testing.T, scenario *probeScenario) (*clashapi.Client, func()) {
	t.Helper()
	return probeClientWithTimeout(t, scenario, 250*time.Millisecond)
}

func probeClientWithTimeout(t *testing.T, scenario *probeScenario, timeout time.Duration) (*clashapi.Client, func()) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer "+fixtureSecret {
			t.Errorf("Authorization = %q", got)
		}
		if scenario.responseDelay > 0 {
			time.Sleep(scenario.responseDelay)
		}
		var status int
		var body string
		switch r.URL.Path {
		case "/version":
			status, body = scenario.versionStatus, scenario.versionBody
		case "/connections":
			status, body = scenario.connectionsStatus, scenario.nextConnectionBody()
		case "/traffic":
			status, body = scenario.trafficStatus, scenario.trafficBody
		case "/memory":
			status, body = scenario.memoryStatus, scenario.memoryBody
		default:
			status, body = http.StatusNotFound, fixtureResponseSentinel
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
	client := newFixtureClient(t, server.URL, server.Client(), timeout, 1<<20)
	return client, server.Close
}

func assertNoProbeLeak(t *testing.T, err error) {
	t.Helper()
	for _, forbidden := range []string{fixtureSecret, fixtureResponseSentinel, "127.0.0.1"} {
		if strings.Contains(err.Error(), forbidden) {
			t.Errorf("Probe() error contains %q: %v", forbidden, err)
		}
	}
}
