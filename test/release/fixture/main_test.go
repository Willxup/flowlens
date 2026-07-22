package main

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"
)

func TestFixtureRequiresBearerAndServesBoundedClashEndpoints(t *testing.T) {
	handler, err := newFixtureHandler(filepath.Join("..", "..", "fixtures", "clashapi"))
	if err != nil {
		t.Fatalf("newFixtureHandler() error = %v", err)
	}

	unauthorized := httptest.NewRecorder()
	handler.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, "/version", nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized status = %d", unauthorized.Code)
	}

	first := fixtureRequest(t, handler, http.MethodGet, "/connections")
	second := fixtureRequest(t, handler, http.MethodGet, "/connections")
	third := fixtureRequest(t, handler, http.MethodGet, "/connections")
	responses := []*httptest.ResponseRecorder{first, second, third}
	totals := make([]struct {
		Upload   int64 `json:"uploadTotal"`
		Download int64 `json:"downloadTotal"`
	}, len(responses))
	for index, response := range responses {
		if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &totals[index]) != nil {
			t.Fatalf("connection snapshot %d is invalid", index)
		}
	}
	if totals[0].Upload >= totals[1].Upload || totals[1].Upload >= totals[2].Upload ||
		totals[0].Download >= totals[1].Download || totals[1].Download >= totals[2].Download {
		t.Fatalf("connection totals are not strictly monotonic: %#v", totals)
	}
	for _, path := range []string{"/version", "/memory"} {
		response := fixtureRequest(t, handler, http.MethodGet, path)
		body, readErr := io.ReadAll(response.Result().Body)
		if readErr != nil || response.Code != http.StatusOK || len(body) == 0 {
			t.Errorf("GET %s = (%d, %d bytes, %v)", path, response.Code, len(body), readErr)
		}
	}
	if response := fixtureRequest(t, handler, http.MethodPost, "/version"); response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST /version status = %d", response.Code)
	}
}

func TestFixtureKeepsTrafficStreamOpenUntilClientCloses(t *testing.T) {
	handler, err := newFixtureHandler(filepath.Join("..", "..", "fixtures", "clashapi"))
	if err != nil {
		t.Fatalf("newFixtureHandler() error = %v", err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	request, _ := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/traffic", nil)
	request.Header.Set("Authorization", "Bearer fixture-clash-secret")
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatalf("GET /traffic error = %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("GET /traffic status = %d", response.StatusCode)
	}

	scanner := bufio.NewScanner(response.Body)
	for sample := 0; sample < 2; sample++ {
		if !scanner.Scan() {
			t.Fatalf("traffic sample %d error = %v", sample, scanner.Err())
		}
	}
	streamResult := make(chan bool, 1)
	go func() { streamResult <- scanner.Scan() }()
	select {
	case <-streamResult:
		t.Fatal("traffic stream closed after the finite fixture samples")
	case <-time.After(50 * time.Millisecond):
	}
	cancel()
	select {
	case more := <-streamResult:
		if more {
			t.Fatal("traffic stream returned data after cancellation")
		}
	case <-time.After(time.Second):
		t.Fatal("traffic stream did not stop after cancellation")
	}
}

func fixtureRequest(t *testing.T, handler http.Handler, method, path string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, path, nil)
	request.Header.Set("Authorization", "Bearer fixture-clash-secret")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}
