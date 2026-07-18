package httpapi

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	flowstatus "github.com/Willxup/flowlens/internal/status"
)

// SessionCookieName is the fixed first-version session cookie.
const SessionCookieName = "flowlens_session"

// Options configures the minimal Stage 1 HTTP boundary.
type Options struct {
	AccessKey  string
	SessionTTL time.Duration
	Status     *flowstatus.Tracker
	Queries    StatisticsQueries
}

// String prevents access-key disclosure through formatting.
func (Options) String() string { return "HTTPOptions{redacted}" }

// GoString prevents access-key disclosure through Go-syntax formatting.
func (options Options) GoString() string { return options.String() }

type handler struct {
	accessKey []byte
	sessions  *SessionStore
	status    *flowstatus.Tracker
	queries   StatisticsQueries
}

// String prevents internal HTTP state disclosure through formatting.
func (*handler) String() string { return "HTTPHandler{redacted}" }

// GoString prevents internal HTTP state disclosure through Go-syntax formatting.
func (h *handler) GoString() string { return h.String() }

// New builds the complete minimal Stage 1 handler.
func New(options Options) (http.Handler, error) {
	if options.AccessKey == "" || options.Status == nil || options.Queries == nil {
		return nil, errors.New("invalid FlowLens HTTP configuration")
	}
	sessions, err := NewSessionStore(options.SessionTTL)
	if err != nil {
		return nil, errors.New("invalid FlowLens HTTP configuration")
	}
	return &handler{
		accessKey: []byte(options.AccessKey), sessions: sessions, status: options.Status, queries: options.Queries,
	}, nil
}

func (h *handler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	setSecurityHeaders(writer.Header())
	switch request.URL.Path {
	case "/healthz":
		h.health(writer, request)
	case "/readyz":
		h.ready(writer, request)
	case "/api/v1/session":
		h.session(writer, request)
	case "/api/v1/status":
		if h.requireSession(writer, request) {
			h.statusResponse(writer, request)
		}
	case "/api/v1/overview":
		if h.requireSession(writer, request) {
			h.overviewResponse(writer, request)
		}
	case "/api/v1/series":
		if h.requireSession(writer, request) {
			h.seriesResponse(writer, request)
		}
	case "/api/v1/quality":
		if h.requireSession(writer, request) {
			h.qualityResponse(writer, request)
		}
	case "/api/v1/storage":
		if h.requireSession(writer, request) {
			h.storageResponse(writer, request)
		}
	default:
		if len(request.URL.Path) >= len("/api/") && request.URL.Path[:len("/api/")] == "/api/" {
			if h.requireSession(writer, request) {
				writer.WriteHeader(http.StatusNotFound)
			}
			return
		}
		writer.WriteHeader(http.StatusNotFound)
	}
}

func (h *handler) health(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		writer.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	writer.WriteHeader(http.StatusNoContent)
}

func (h *handler) ready(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		writer.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if !h.status.Snapshot().Ready {
		writer.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	writer.WriteHeader(http.StatusNoContent)
}

func (h *handler) session(writer http.ResponseWriter, request *http.Request) {
	switch request.Method {
	case http.MethodPost:
		h.login(writer, request)
	case http.MethodDelete:
		if h.requireSession(writer, request) {
			h.logout(writer, request)
		}
	default:
		writer.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (h *handler) login(writer http.ResponseWriter, request *http.Request) {
	if !sameOrigin(request.Header.Get("Origin"), request.Host) {
		writer.WriteHeader(http.StatusForbidden)
		return
	}
	if !isJSON(request.Header.Get("Content-Type")) {
		writer.WriteHeader(http.StatusUnsupportedMediaType)
		return
	}
	request.Body = http.MaxBytesReader(writer, request.Body, maxLoginBodyBytes)
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	var payload struct {
		AccessKey string `json:"access_key"`
	}
	if err := decoder.Decode(&payload); err != nil {
		var sizeError *http.MaxBytesError
		if errors.As(err, &sizeError) {
			writer.WriteHeader(http.StatusRequestEntityTooLarge)
		} else {
			writer.WriteHeader(http.StatusBadRequest)
		}
		return
	}
	if err := requireJSONEOF(decoder); err != nil {
		writer.WriteHeader(http.StatusBadRequest)
		return
	}
	if subtle.ConstantTimeCompare([]byte(payload.AccessKey), h.accessKey) != 1 {
		writer.WriteHeader(http.StatusUnauthorized)
		return
	}
	id, err := h.sessions.Create(time.Now())
	if err != nil {
		writer.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	http.SetCookie(writer, &http.Cookie{
		Name:     SessionCookieName,
		Value:    id,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	})
	writer.WriteHeader(http.StatusNoContent)
}

func requireJSONEOF(decoder *json.Decoder) error {
	var trailing any
	err := decoder.Decode(&trailing)
	if errors.Is(err, io.EOF) {
		return nil
	}
	return errors.New("unexpected trailing JSON")
}

func (h *handler) logout(writer http.ResponseWriter, request *http.Request) {
	if !sameOrigin(request.Header.Get("Origin"), request.Host) {
		writer.WriteHeader(http.StatusForbidden)
		return
	}
	cookie, _ := request.Cookie(SessionCookieName)
	h.sessions.Delete(cookie.Value)
	http.SetCookie(writer, &http.Cookie{
		Name:     SessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   -1,
		Expires:  time.Unix(1, 0).UTC(),
	})
	writer.WriteHeader(http.StatusNoContent)
}

func (h *handler) requireSession(writer http.ResponseWriter, request *http.Request) bool {
	cookie, err := request.Cookie(SessionCookieName)
	if err != nil || !h.sessions.Valid(cookie.Value, time.Now()) {
		writer.WriteHeader(http.StatusUnauthorized)
		return false
	}
	return true
}

func (h *handler) statusResponse(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		writer.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	snapshot := h.status.Snapshot()
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprintf(writer, `{"status":%q,"reason":%q}`, snapshot.Level, snapshot.Reason)
}
