package httpapi

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"strings"
	"time"

	"github.com/Willxup/flowlens/internal/clashapi"
	"github.com/Willxup/flowlens/internal/collector"
	flowstatus "github.com/Willxup/flowlens/internal/status"
)

// SessionCookieName is the fixed first-version session cookie.
const SessionCookieName = "flowlens_session"

// Options configures the minimal Stage 1 HTTP boundary.
type Options struct {
	AccessKey        string
	SessionTTL       time.Duration
	Status           *flowstatus.Tracker
	Queries          StatisticsQueries
	CapabilitySource CapabilitySource
	Web              fs.FS
	Live             LiveSource
	Timezone         string
}

// CapabilitySource exposes only the public optional dimension matrix.
type CapabilitySource interface {
	Capabilities() clashapi.DimensionCapabilities
}

// String prevents access-key disclosure through formatting.
func (Options) String() string { return "HTTPOptions{redacted}" }

// GoString prevents access-key disclosure through Go-syntax formatting.
func (options Options) GoString() string { return options.String() }

type handler struct {
	accessKey    []byte
	sessions     *SessionStore
	status       *flowstatus.Tracker
	queries      StatisticsQueries
	capabilities CapabilitySource
	web          fs.FS
	live         LiveSource
	timezone     string
}

// LiveSource exposes the immutable one-second window needed by SSE.
type LiveSource interface {
	Snapshot() []collector.SpeedSample
}

// String prevents internal HTTP state disclosure through formatting.
func (*handler) String() string { return "HTTPHandler{redacted}" }

// GoString prevents internal HTTP state disclosure through Go-syntax formatting.
func (h *handler) GoString() string { return h.String() }

// New builds the complete minimal Stage 1 handler.
func New(options Options) (http.Handler, error) {
	if options.AccessKey == "" || options.Status == nil || options.Queries == nil || options.CapabilitySource == nil ||
		options.Timezone == "" {
		return nil, errors.New("invalid FlowLens HTTP configuration")
	}
	sessions, err := NewSessionStore(options.SessionTTL)
	if err != nil {
		return nil, errors.New("invalid FlowLens HTTP configuration")
	}
	return &handler{
		accessKey: []byte(options.AccessKey), sessions: sessions, status: options.Status, queries: options.Queries,
		capabilities: options.CapabilitySource, web: options.Web, live: options.Live, timezone: options.Timezone,
	}, nil
}

func (h *handler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	if strings.HasPrefix(request.URL.Path, "/api/") {
		writer = &apiResponseWriter{ResponseWriter: writer}
	}
	setSecurityHeaders(writer.Header())
	if h.webResponse(writer, request) {
		return
	}
	if strings.HasPrefix(request.URL.Path, "/api/v1/labels/") {
		if h.requireSession(writer, request) {
			h.labelItemResponse(writer, request)
		}
		return
	}
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
	case "/api/v1/live":
		if h.requireSession(writer, request) {
			h.liveResponse(writer, request)
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
	case "/api/v1/breakdown":
		if h.requireSession(writer, request) {
			h.breakdownResponse(writer, request)
		}
	case "/api/v1/connections/live":
		if h.requireSession(writer, request) {
			h.liveTargetsResponse(writer, request)
		}
	case "/api/v1/runtime-sessions":
		if h.requireSession(writer, request) {
			h.runtimeSessionsResponse(writer, request)
		}
	case "/api/v1/labels":
		if h.requireSession(writer, request) {
			h.labelsResponse(writer, request)
		}
	case "/api/v1/label-candidates":
		if h.requireSession(writer, request) {
			h.labelCandidatesResponse(writer, request)
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
	if !h.validSession(request) {
		writer.WriteHeader(http.StatusUnauthorized)
		return false
	}
	return true
}

func (h *handler) validSession(request *http.Request) bool {
	cookie, err := request.Cookie(SessionCookieName)
	return err == nil && h.sessions.Valid(cookie.Value, time.Now())
}

func (h *handler) statusResponse(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		writer.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	snapshot := h.status.Snapshot()
	capabilities := h.capabilities.Capabilities()
	writeJSON(writer, struct {
		Status       flowstatus.Level `json:"status"`
		Reason       string           `json:"reason"`
		Timezone     string           `json:"timezone"`
		Capabilities struct {
			ConnectionID bool `json:"connection_id"`
			Source       bool `json:"source"`
			Destination  bool `json:"destination"`
			Port         bool `json:"port"`
			Network      bool `json:"network"`
			Domain       bool `json:"domain"`
		} `json:"capabilities"`
	}{
		Status: snapshot.Level, Reason: snapshot.Reason, Timezone: h.timezone,
		Capabilities: struct {
			ConnectionID bool `json:"connection_id"`
			Source       bool `json:"source"`
			Destination  bool `json:"destination"`
			Port         bool `json:"port"`
			Network      bool `json:"network"`
			Domain       bool `json:"domain"`
		}{
			ConnectionID: capabilities.ConnectionID, Source: capabilities.SourceIP,
			Destination: capabilities.DestinationIP, Port: capabilities.DestinationPort,
			Network: capabilities.Network, Domain: capabilities.Host,
		},
	})
}
