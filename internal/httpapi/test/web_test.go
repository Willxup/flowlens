package httpapi_test

import (
	"io/fs"
	"net/http"
	"testing"
	"testing/fstest"
	"time"

	"github.com/Willxup/flowlens/internal/httpapi"
	flowstatus "github.com/Willxup/flowlens/internal/status"
)

func TestWebRoutesSeparatePublicAssetsAndProtectedPages(t *testing.T) {
	handler := webHandler(t)

	login := request(t, handler, http.MethodGet, "/login", "", nil)
	if login.Code != http.StatusOK || login.Body.String() != "<!doctype html><title>FlowLens</title>" {
		t.Fatalf("GET /login = %d %q", login.Code, login.Body.String())
	}
	if got := login.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("login Cache-Control = %q", got)
	}
	if got := login.Header().Get("Content-Security-Policy"); got != httpapi.WebContentSecurityPolicy {
		t.Errorf("login CSP = %q", got)
	}

	for _, path := range []string{"/assets/app-a1b2c3.js", "/theme-init.js", "/favicon.svg"} {
		response := request(t, handler, http.MethodGet, path, "", nil)
		if response.Code != http.StatusOK {
			t.Errorf("GET %s = %d", path, response.Code)
		}
	}
	asset := request(t, handler, http.MethodGet, "/assets/app-a1b2c3.js", "", nil)
	if got := asset.Header().Get("Cache-Control"); got != "public, max-age=31536000, immutable" {
		t.Errorf("asset Cache-Control = %q", got)
	}
	if got := asset.Header().Get("Content-Type"); got != "text/javascript; charset=utf-8" {
		t.Errorf("asset Content-Type = %q", got)
	}

	root := request(t, handler, http.MethodGet, "/", "", nil)
	if root.Code != http.StatusFound || root.Header().Get("Location") != "/login" {
		t.Fatalf("anonymous root = %d Location=%q", root.Code, root.Header().Get("Location"))
	}
	cookie := loginCookie(t, handler)
	for _, path := range []string{"/", "/traffic", "/targets", "/storage"} {
		response := request(t, handler, http.MethodGet, path, "", cookie)
		if response.Code != http.StatusOK || response.Body.String() != login.Body.String() {
			t.Errorf("authenticated GET %s = %d %q", path, response.Code, response.Body.String())
		}
	}
}

func TestWebRoutesRejectUnknownAssetsMethodsAndDirectories(t *testing.T) {
	handler := webHandler(t)

	for _, path := range []string{"/assets/", "/assets/missing.js", "/favicon.ico", "/unknown"} {
		response := request(t, handler, http.MethodGet, path, "", nil)
		if response.Code != http.StatusNotFound {
			t.Errorf("GET %s = %d, want 404", path, response.Code)
		}
	}
	if response := request(t, handler, http.MethodPost, "/login", "", nil); response.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST /login = %d", response.Code)
	}
	head := request(t, handler, http.MethodHead, "/login", "", nil)
	if head.Code != http.StatusOK || head.Body.Len() != 0 {
		t.Errorf("HEAD /login = %d body=%q", head.Code, head.Body.String())
	}
}

func TestDisabledAuthenticationServesDashboardAndLeavesLogin(t *testing.T) {
	handler := webHandlerWithOptions(t, httpapi.Options{AuthDisabled: true})

	root := request(t, handler, http.MethodGet, "/", "", nil)
	if root.Code != http.StatusOK || root.Body.String() != "<!doctype html><title>FlowLens</title>" {
		t.Fatalf("anonymous root = %d %q", root.Code, root.Body.String())
	}
	login := request(t, handler, http.MethodGet, "/login", "", nil)
	if login.Code != http.StatusFound || login.Header().Get("Location") != "/" {
		t.Fatalf("disabled login = %d Location=%q", login.Code, login.Header().Get("Location"))
	}
}

func webHandler(t *testing.T) http.Handler {
	return webHandlerWithOptions(t, httpapi.Options{})
}

func webHandlerWithOptions(t *testing.T, options httpapi.Options) http.Handler {
	t.Helper()
	content := fstest.MapFS{
		"index.html":             &fstest.MapFile{Data: []byte("<!doctype html><title>FlowLens</title>")},
		"theme-init.js":          &fstest.MapFile{Data: []byte("document.documentElement.dataset.theme='light'")},
		"favicon.svg":            &fstest.MapFile{Data: []byte("<svg xmlns=\"http://www.w3.org/2000/svg\"/>")},
		"assets/app-a1b2c3.js":   &fstest.MapFile{Data: []byte("console.log('FlowLens')")},
		"assets/ignored/data.js": &fstest.MapFile{Mode: fs.ModeDir},
	}
	options.AccessKey = fixtureAccessKey
	options.SessionTTL = time.Hour
	options.Status = flowstatus.NewTracker()
	options.Queries = fixtureStatisticsQueries()
	options.CapabilitySource = fixtureCapabilitySource{}
	options.Web = content
	options.Timezone = "UTC"
	handler, err := httpapi.New(options)
	if err != nil {
		t.Fatalf("httpapi.New() error = %v", err)
	}
	return handler
}
