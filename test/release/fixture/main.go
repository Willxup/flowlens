package main

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"
)

const fixtureSecret = "fixture-clash-secret"

type fixtureHandler struct {
	version     []byte
	connections [2]fixtureConnectionsSnapshot
	traffic     []byte
	memory      []byte
	next        atomic.Uint64
}

type fixtureConnectionsSnapshot struct {
	DownloadTotal int64               `json:"downloadTotal"`
	UploadTotal   int64               `json:"uploadTotal"`
	Connections   []fixtureConnection `json:"connections"`
	Memory        int64               `json:"memory"`
}

type fixtureConnection struct {
	ID          string          `json:"id"`
	Metadata    json.RawMessage `json:"metadata"`
	Upload      int64           `json:"upload"`
	Download    int64           `json:"download"`
	Start       string          `json:"start"`
	Chains      []string        `json:"chains"`
	Rule        string          `json:"rule"`
	RulePayload string          `json:"rulePayload"`
}

func newFixtureHandler(directory string) (*fixtureHandler, error) {
	read := func(name string) ([]byte, error) {
		contents, err := os.ReadFile(filepath.Join(directory, name))
		if err != nil || len(contents) == 0 {
			return nil, errors.New("fixture data is unavailable")
		}
		return contents, nil
	}
	handler := &fixtureHandler{}
	readConnections := func(name string) (fixtureConnectionsSnapshot, error) {
		contents, err := read(name)
		if err != nil {
			return fixtureConnectionsSnapshot{}, err
		}
		var snapshot fixtureConnectionsSnapshot
		if json.Unmarshal(contents, &snapshot) != nil || len(snapshot.Connections) == 0 {
			return fixtureConnectionsSnapshot{}, errors.New("fixture data is unavailable")
		}
		return snapshot, nil
	}
	var err error
	if handler.version, err = read("version.json"); err != nil {
		return nil, err
	}
	if handler.connections[0], err = readConnections("connections-1.json"); err != nil {
		return nil, err
	}
	if handler.connections[1], err = readConnections("connections-2.json"); err != nil {
		return nil, err
	}
	if handler.traffic, err = read("traffic.ndjson"); err != nil {
		return nil, err
	}
	if handler.memory, err = read("memory.ndjson"); err != nil {
		return nil, err
	}
	return handler, nil
}

func (handler *fixtureHandler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	if subtle.ConstantTimeCompare([]byte(request.Header.Get("Authorization")), []byte("Bearer "+fixtureSecret)) != 1 {
		writer.WriteHeader(http.StatusUnauthorized)
		return
	}
	if request.Method != http.MethodGet {
		writer.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var contents []byte
	switch request.URL.Path {
	case "/version":
		writer.Header().Set("Content-Type", "application/json")
		contents = handler.version
	case "/connections":
		writer.Header().Set("Content-Type", "application/json")
		writer.Header().Set("Cache-Control", "no-store")
		writer.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(writer).Encode(handler.nextConnections())
		return
	case "/traffic":
		handler.serveTraffic(writer, request)
		return
	case "/memory":
		writer.Header().Set("Content-Type", "application/x-ndjson")
		contents = handler.memory
	default:
		writer.WriteHeader(http.StatusNotFound)
		return
	}
	writer.Header().Set("Cache-Control", "no-store")
	writer.WriteHeader(http.StatusOK)
	_, _ = writer.Write(contents)
}

func (handler *fixtureHandler) nextConnections() fixtureConnectionsSnapshot {
	step := handler.next.Add(1) - 1
	if step == 0 {
		return handler.connections[0]
	}
	snapshot := handler.connections[1]
	snapshot.Connections = append([]fixtureConnection(nil), snapshot.Connections...)
	growth := int64(step - 1)
	snapshot.UploadTotal += 250 * growth
	snapshot.DownloadTotal += 750 * growth
	if len(snapshot.Connections) > 0 {
		snapshot.Connections[0].Upload += 150 * growth
		snapshot.Connections[0].Download += 500 * growth
	}
	if len(snapshot.Connections) > 1 {
		snapshot.Connections[1].Upload += 100 * growth
		snapshot.Connections[1].Download += 250 * growth
	}
	return snapshot
}

func (handler *fixtureHandler) serveTraffic(writer http.ResponseWriter, request *http.Request) {
	writer.Header().Set("Content-Type", "application/x-ndjson")
	writer.Header().Set("Cache-Control", "no-store")
	writer.WriteHeader(http.StatusOK)
	if _, err := writer.Write(handler.traffic); err != nil {
		return
	}
	if flusher, ok := writer.(http.Flusher); ok {
		flusher.Flush()
	}
	<-request.Context().Done()
}

func main() {
	handler, err := newFixtureHandler("/fixtures")
	if err != nil {
		log.Fatal(err)
	}
	server := &http.Server{
		Addr: ":9090", Handler: handler,
		ReadHeaderTimeout: 3 * time.Second,
		IdleTimeout:       30 * time.Second,
	}
	if err := server.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
