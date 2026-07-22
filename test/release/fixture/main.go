package main

import (
	"crypto/subtle"
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
	connections [2][]byte
	traffic     []byte
	memory      []byte
	next        atomic.Uint64
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
	var err error
	if handler.version, err = read("version.json"); err != nil {
		return nil, err
	}
	if handler.connections[0], err = read("connections-1.json"); err != nil {
		return nil, err
	}
	if handler.connections[1], err = read("connections-2.json"); err != nil {
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
		index := (handler.next.Add(1) - 1) % uint64(len(handler.connections))
		contents = handler.connections[index]
	case "/traffic":
		writer.Header().Set("Content-Type", "application/x-ndjson")
		contents = handler.traffic
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
