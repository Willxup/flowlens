package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
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
	if first.Code != http.StatusOK || second.Code != http.StatusOK || first.Body.String() == second.Body.String() {
		t.Fatalf("connection snapshots = (%d, %d, equal=%t)", first.Code, second.Code, first.Body.String() == second.Body.String())
	}
	for _, path := range []string{"/version", "/traffic", "/memory"} {
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

func fixtureRequest(t *testing.T, handler http.Handler, method, path string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, path, nil)
	request.Header.Set("Authorization", "Bearer fixture-clash-secret")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}
