package httpapi_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Willxup/flowlens/internal/clashapi"
	"github.com/Willxup/flowlens/internal/httpapi"
	flowstatus "github.com/Willxup/flowlens/internal/status"
)

const fixtureAccessKey = "fixture-access-key-123456"

func TestPublicHealthAndReadinessEndpoints(t *testing.T) {
	tracker := flowstatus.NewTracker()
	handler := newHandler(t, tracker)
	assertResponse(t, handler, http.MethodGet, "/healthz", "", nil, http.StatusNoContent)
	assertResponse(t, handler, http.MethodGet, "/readyz", "", nil, http.StatusServiceUnavailable)
	if err := tracker.Set(flowstatus.LevelOK, "ready", true); err != nil {
		t.Fatalf("status Set() error = %v", err)
	}
	assertResponse(t, handler, http.MethodGet, "/readyz", "", nil, http.StatusNoContent)
}

func TestLoginStatusAndLogoutFlow(t *testing.T) {
	tracker := flowstatus.NewTracker()
	if err := tracker.Set(flowstatus.LevelOK, "ready", true); err != nil {
		t.Fatalf("status Set() error = %v", err)
	}
	handler := newHandler(t, tracker)
	assertResponse(t, handler, http.MethodGet, "/api/v1/status", "", nil, http.StatusUnauthorized)

	login := request(t, handler, http.MethodPost, "/api/v1/session", `{"access_key":"`+fixtureAccessKey+`"}`, nil)
	if login.Code != http.StatusNoContent {
		t.Fatalf("login status = %d, body=%q", login.Code, login.Body.String())
	}
	cookies := login.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("login cookies = %#v", cookies)
	}
	cookie := cookies[0]
	if cookie.Name != httpapi.SessionCookieName || cookie.Value == "" || !cookie.HttpOnly || cookie.Secure ||
		cookie.SameSite != http.SameSiteStrictMode || cookie.Path != "/" {
		t.Errorf("session cookie = %#v", cookie)
	}

	statusResponse := request(t, handler, http.MethodGet, "/api/v1/status", "", cookie)
	if statusResponse.Code != http.StatusOK {
		t.Fatalf("status code = %d, body=%q", statusResponse.Code, statusResponse.Body.String())
	}
	var body struct {
		Status       string          `json:"status"`
		Reason       string          `json:"reason"`
		Capabilities map[string]bool `json:"capabilities"`
	}
	if err := json.Unmarshal(statusResponse.Body.Bytes(), &body); err != nil {
		t.Fatalf("status JSON: %v", err)
	}
	if body.Status != "ok" || body.Reason != "ready" || len(body.Capabilities) != 6 || !body.Capabilities["connection_id"] {
		t.Errorf("status body = %#v", body)
	}

	logout := request(t, handler, http.MethodDelete, "/api/v1/session", "", cookie)
	if logout.Code != http.StatusNoContent {
		t.Fatalf("logout status = %d, body=%q", logout.Code, logout.Body.String())
	}
	cleared := logout.Result().Cookies()
	if len(cleared) != 1 || cleared[0].Name != httpapi.SessionCookieName || cleared[0].Value != "" || cleared[0].MaxAge >= 0 {
		t.Errorf("logout cookie = %#v", cleared)
	}
	assertResponse(t, handler, http.MethodGet, "/api/v1/status", "", cookie, http.StatusUnauthorized)
}

func TestWrongAccessKeysReturnUniformUnauthorized(t *testing.T) {
	handler := newHandler(t, flowstatus.NewTracker())
	var firstBody string
	for index, key := range []string{"wrong", "another-completely-wrong-key"} {
		response := request(t, handler, http.MethodPost, "/api/v1/session", `{"access_key":"`+key+`"}`, nil)
		if response.Code != http.StatusUnauthorized {
			t.Errorf("wrong key status = %d", response.Code)
		}
		if index == 0 {
			firstBody = response.Body.String()
		} else if response.Body.String() != firstBody {
			t.Errorf("wrong-key bodies differ: %q and %q", firstBody, response.Body.String())
		}
		if strings.Contains(response.Body.String(), key) || strings.Contains(response.Body.String(), fixtureAccessKey) {
			t.Errorf("wrong-key body leaked a key: %q", response.Body.String())
		}
	}
}

func TestBusinessRoutesAuthenticateBeforeRouteDisclosure(t *testing.T) {
	handler := newHandler(t, flowstatus.NewTracker())
	assertResponse(t, handler, http.MethodGet, "/api/v1/not-implemented", "", nil, http.StatusUnauthorized)
	cookie := loginCookie(t, handler)
	assertResponse(t, handler, http.MethodGet, "/api/v1/not-implemented", "", cookie, http.StatusNotFound)
}

func TestLoginRejectsUnsafeOrInvalidRequests(t *testing.T) {
	handler := newHandler(t, flowstatus.NewTracker())
	tests := []struct {
		name        string
		method      string
		body        string
		origin      string
		host        string
		contentType string
		want        int
	}{
		{name: "wrong method", method: http.MethodGet, origin: "http://example.test", host: "example.test", contentType: "application/json", want: http.StatusMethodNotAllowed},
		{name: "missing origin", method: http.MethodPost, body: `{"access_key":"x"}`, host: "example.test", contentType: "application/json", want: http.StatusForbidden},
		{name: "mismatched origin", method: http.MethodPost, body: `{"access_key":"x"}`, origin: "http://other.example.test", host: "example.test", contentType: "application/json", want: http.StatusForbidden},
		{name: "invalid host", method: http.MethodPost, body: `{"access_key":"x"}`, origin: "http://example.test", host: "bad host", contentType: "application/json", want: http.StatusForbidden},
		{name: "wrong content type", method: http.MethodPost, body: `{"access_key":"x"}`, origin: "http://example.test", host: "example.test", contentType: "text/plain", want: http.StatusUnsupportedMediaType},
		{name: "malformed JSON", method: http.MethodPost, body: `{`, origin: "http://example.test", host: "example.test", contentType: "application/json", want: http.StatusBadRequest},
		{name: "trailing JSON", method: http.MethodPost, body: `{"access_key":"x"}{}`, origin: "http://example.test", host: "example.test", contentType: "application/json", want: http.StatusBadRequest},
		{name: "unknown field", method: http.MethodPost, body: `{"access_key":"x","extra":true}`, origin: "http://example.test", host: "example.test", contentType: "application/json", want: http.StatusBadRequest},
		{name: "oversized", method: http.MethodPost, body: `{"access_key":"` + strings.Repeat("x", 4096) + `"}`, origin: "http://example.test", host: "example.test", contentType: "application/json", want: http.StatusRequestEntityTooLarge},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			req := httptest.NewRequest(test.method, "http://example.test/api/v1/session", strings.NewReader(test.body))
			req.Host = test.host
			if test.origin != "" {
				req.Header.Set("Origin", test.origin)
			}
			if test.contentType != "" {
				req.Header.Set("Content-Type", test.contentType)
			}
			handler.ServeHTTP(recorder, req)
			if recorder.Code != test.want {
				t.Errorf("status = %d, want %d, body=%q", recorder.Code, test.want, recorder.Body.String())
			}
		})
	}
}

func TestHandlerAddsSecurityHeadersAndFormattingRedactsAccessKey(t *testing.T) {
	tracker := flowstatus.NewTracker()
	handler, err := httpapi.New(httpapi.Options{
		AccessKey: fixtureAccessKey, SessionTTL: time.Hour, Status: tracker, Queries: fixtureStatisticsQueries(),
		CapabilitySource: fixtureCapabilitySource{},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	response := request(t, handler, http.MethodGet, "/healthz", "", nil)
	for header, want := range map[string]string{
		"X-Content-Type-Options":  "nosniff",
		"Referrer-Policy":         "no-referrer",
		"Content-Security-Policy": "default-src 'none'; frame-ancestors 'none'",
	} {
		if got := response.Header().Get(header); got != want {
			t.Errorf("%s = %q, want %q", header, got, want)
		}
	}
	options := httpapi.Options{
		AccessKey: fixtureAccessKey, SessionTTL: time.Hour, Status: tracker, Queries: fixtureStatisticsQueries(),
		CapabilitySource: fixtureCapabilitySource{},
	}
	for _, value := range []any{options, handler} {
		for _, format := range []string{"%v", "%+v", "%#v"} {
			if formatted := fmt.Sprintf(format, value); strings.Contains(formatted, fixtureAccessKey) {
				t.Errorf("fmt.Sprintf(%q) leaked access key: %s", format, formatted)
			}
		}
	}
}

func newHandler(t *testing.T, tracker *flowstatus.Tracker) http.Handler {
	t.Helper()
	handler, err := httpapi.New(httpapi.Options{
		AccessKey: fixtureAccessKey, SessionTTL: time.Hour, Status: tracker, Queries: fixtureStatisticsQueries(),
		CapabilitySource: fixtureCapabilitySource{},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	return handler
}

type fixtureCapabilitySource struct{}

func (fixtureCapabilitySource) Capabilities() clashapi.DimensionCapabilities {
	return clashapi.DimensionCapabilities{
		ConnectionID: true, SourceIP: true, DestinationIP: true,
		DestinationPort: true, Network: true, Host: true,
	}
}

func loginCookie(t *testing.T, handler http.Handler) *http.Cookie {
	t.Helper()
	response := request(t, handler, http.MethodPost, "/api/v1/session", `{"access_key":"`+fixtureAccessKey+`"}`, nil)
	if response.Code != http.StatusNoContent || len(response.Result().Cookies()) != 1 {
		t.Fatalf("login = status:%d cookies:%#v body:%q", response.Code, response.Result().Cookies(), response.Body.String())
	}
	return response.Result().Cookies()[0]
}

func assertResponse(t *testing.T, handler http.Handler, method, path, body string, cookie *http.Cookie, want int) {
	t.Helper()
	response := request(t, handler, method, path, body, cookie)
	if response.Code != want {
		t.Errorf("%s %s status = %d, want %d, body=%q", method, path, response.Code, want, response.Body.String())
	}
}

func request(t *testing.T, handler http.Handler, method, path, body string, cookie *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(method, "http://example.test"+path, bytes.NewBufferString(body))
	req.Host = "example.test"
	if method == http.MethodPost || method == http.MethodDelete || method == http.MethodPut {
		req.Header.Set("Origin", "http://example.test")
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if cookie != nil {
		req.AddCookie(cookie)
	}
	handler.ServeHTTP(recorder, req)
	return recorder
}
