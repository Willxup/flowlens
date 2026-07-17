package clashapi_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Willxup/flowlens/internal/clashapi"
)

func TestTrafficStreamReturnsOrderedSamples(t *testing.T) {
	server := streamFixtureServer(t, "/traffic", "traffic.ndjson")
	defer server.Close()
	client := newFixtureClient(t, server.URL, server.Client(), time.Second, 1<<20)

	stream, err := client.Traffic(context.Background())
	if err != nil {
		t.Fatalf("Traffic() error = %v", err)
	}
	defer stream.Close()

	first, err := stream.Next()
	if err != nil {
		t.Fatalf("first Next() error = %v", err)
	}
	second, err := stream.Next()
	if err != nil {
		t.Fatalf("second Next() error = %v", err)
	}
	if first.Up != 128 || first.Down != 512 {
		t.Errorf("first sample = %#v", first)
	}
	if second.Up != 256 || second.Down != 1024 {
		t.Errorf("second sample = %#v", second)
	}
	if _, err := stream.Next(); !errors.Is(err, io.EOF) {
		t.Errorf("final Next() error = %v, want EOF", err)
	}
}

func TestMemoryStreamReturnsSample(t *testing.T) {
	server := streamFixtureServer(t, "/memory", "memory.ndjson")
	defer server.Close()
	client := newFixtureClient(t, server.URL, server.Client(), time.Second, 1<<20)

	stream, err := client.Memory(context.Background())
	if err != nil {
		t.Fatalf("Memory() error = %v", err)
	}
	defer stream.Close()
	sample, err := stream.Next()
	if err != nil {
		t.Fatalf("Next() error = %v", err)
	}
	if sample.Inuse != 67108864 || sample.OSLimit != 0 {
		t.Errorf("sample = %#v", sample)
	}
}

func TestTrafficStreamSkipsBlankLines(t *testing.T) {
	client, closeServer := trafficClientForBody(t, "\n  \n{\"up\":1,\"down\":2}\n", 1024)
	defer closeServer()
	stream, err := client.Traffic(context.Background())
	if err != nil {
		t.Fatalf("Traffic() error = %v", err)
	}
	defer stream.Close()

	sample, err := stream.Next()
	if err != nil {
		t.Fatalf("Next() error = %v", err)
	}
	if sample.Up != 1 || sample.Down != 2 {
		t.Errorf("sample = %#v", sample)
	}
}

func TestTrafficStreamSurvivesOpeningTimeoutAfterHeaders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		time.Sleep(60 * time.Millisecond)
		_, _ = io.WriteString(w, `{"up":3,"down":4}`+"\n")
		w.(http.Flusher).Flush()
	}))
	defer server.Close()
	client := newFixtureClient(t, server.URL, server.Client(), 20*time.Millisecond, 1024)

	stream, err := client.Traffic(context.Background())
	if err != nil {
		t.Fatalf("Traffic() error = %v", err)
	}
	defer stream.Close()
	sample, err := stream.Next()
	if err != nil {
		t.Fatalf("Next() after opening timeout error = %v", err)
	}
	if sample.Up != 3 || sample.Down != 4 {
		t.Errorf("sample = %#v", sample)
	}
}

func TestTrafficHonorsOpeningTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer server.Close()
	client := newFixtureClient(t, server.URL, server.Client(), 20*time.Millisecond, 1024)

	started := time.Now()
	_, err := client.Traffic(context.Background())
	if err == nil {
		t.Fatal("Traffic() error = nil")
	}
	if elapsed := time.Since(started); elapsed > 500*time.Millisecond {
		t.Fatalf("Traffic() elapsed = %v", elapsed)
	}
	assertSanitized(t, err)
}

func TestTrafficStreamCloseCancelsAndIsIdempotent(t *testing.T) {
	requestCanceled := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		<-r.Context().Done()
		close(requestCanceled)
	}))
	defer server.Close()
	client := newFixtureClient(t, server.URL, server.Client(), time.Second, 1024)
	stream, err := client.Traffic(context.Background())
	if err != nil {
		t.Fatalf("Traffic() error = %v", err)
	}

	if err := stream.Close(); err != nil {
		t.Fatalf("first Close() error = %v", err)
	}
	if err := stream.Close(); err != nil {
		t.Fatalf("second Close() error = %v", err)
	}
	select {
	case <-requestCanceled:
	case <-time.After(time.Second):
		t.Fatal("request context was not canceled")
	}
}

func TestTrafficStreamCallerCancellationInterruptsNext(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer server.Close()
	client := newFixtureClient(t, server.URL, server.Client(), time.Second, 1024)
	ctx, cancel := context.WithCancel(context.Background())
	stream, err := client.Traffic(ctx)
	if err != nil {
		t.Fatalf("Traffic() error = %v", err)
	}
	defer stream.Close()
	cancel()

	if _, err := stream.Next(); err == nil {
		t.Fatal("Next() error = nil")
	} else {
		assertSanitized(t, err)
	}
}

func TestTrafficStreamRejectsNegativeSample(t *testing.T) {
	for name, body := range map[string]string{
		"up":   `{"up":-1,"down":0}` + "\n",
		"down": `{"up":0,"down":-1}` + "\n",
	} {
		t.Run(name, func(t *testing.T) {
			client, closeServer := trafficClientForBody(t, body, 1024)
			defer closeServer()
			stream, err := client.Traffic(context.Background())
			if err != nil {
				t.Fatalf("Traffic() error = %v", err)
			}
			defer stream.Close()
			if _, err := stream.Next(); err == nil {
				t.Fatal("Next() error = nil")
			} else {
				assertSanitized(t, err)
			}
		})
	}
}

func TestTrafficStreamRejectsMissingFields(t *testing.T) {
	for name, body := range map[string]string{
		"up":   `{"down":0}` + "\n",
		"down": `{"up":0}` + "\n",
	} {
		t.Run(name, func(t *testing.T) {
			client, closeServer := trafficClientForBody(t, body, 1024)
			defer closeServer()
			stream, err := client.Traffic(context.Background())
			if err != nil {
				t.Fatalf("Traffic() error = %v", err)
			}
			defer stream.Close()
			if _, err := stream.Next(); err == nil {
				t.Fatal("Next() error = nil")
			} else {
				assertSanitized(t, err)
			}
		})
	}
}

func TestMemoryStreamRejectsNegativeAndOverflowSamples(t *testing.T) {
	for name, body := range map[string]string{
		"negative inuse": `{"inuse":-1,"oslimit":0}` + "\n",
		"negative limit": `{"inuse":0,"oslimit":-1}` + "\n",
		"int64 overflow": `{"inuse":9223372036854775808,"oslimit":0}` + "\n",
	} {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.WriteString(w, body)
			}))
			defer server.Close()
			client := newFixtureClient(t, server.URL, server.Client(), time.Second, 1024)
			stream, err := client.Memory(context.Background())
			if err != nil {
				t.Fatalf("Memory() error = %v", err)
			}
			defer stream.Close()
			if _, err := stream.Next(); err == nil {
				t.Fatal("Next() error = nil")
			} else {
				assertSanitized(t, err)
			}
		})
	}
}

func TestMemoryStreamRejectsMissingInuse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"oslimit":0}`+"\n")
	}))
	defer server.Close()
	client := newFixtureClient(t, server.URL, server.Client(), time.Second, 1024)
	stream, err := client.Memory(context.Background())
	if err != nil {
		t.Fatalf("Memory() error = %v", err)
	}
	defer stream.Close()

	if _, err := stream.Next(); err == nil {
		t.Fatal("Next() error = nil")
	} else {
		assertSanitized(t, err)
	}
}

func TestTrafficStreamRejectsOversizedToken(t *testing.T) {
	body := `{"up":1,"down":2,"padding":"` + fixtureResponseSentinel + strings.Repeat("x", 64) + `"}` + "\n"
	client, closeServer := trafficClientForBody(t, body, 32)
	defer closeServer()
	stream, err := client.Traffic(context.Background())
	if err != nil {
		t.Fatalf("Traffic() error = %v", err)
	}
	defer stream.Close()

	if _, err := stream.Next(); err == nil {
		t.Fatal("Next() error = nil")
	} else {
		assertSanitized(t, err)
	}
}

func TestTrafficStreamRejectsInvalidJSONWithoutReturningBody(t *testing.T) {
	client, closeServer := trafficClientForBody(t, `{"up":"`+fixtureResponseSentinel+`"`+"\n", 1024)
	defer closeServer()
	stream, err := client.Traffic(context.Background())
	if err != nil {
		t.Fatalf("Traffic() error = %v", err)
	}
	defer stream.Close()

	if _, err := stream.Next(); err == nil {
		t.Fatal("Next() error = nil")
	} else {
		assertSanitized(t, err)
	}
}

func TestTrafficRejectsNonSuccessWithoutReturningBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, fixtureResponseSentinel)
	}))
	defer server.Close()
	client := newFixtureClient(t, server.URL, server.Client(), time.Second, 1024)

	_, err := client.Traffic(context.Background())
	if err == nil {
		t.Fatal("Traffic() error = nil")
	}
	assertSanitized(t, err)
}

func streamFixtureServer(t *testing.T, wantPath, fixtureName string) *httptest.Server {
	t.Helper()
	body := fixture(t, fixtureName)
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != wantPath {
			t.Errorf("request path = %q, want %q", r.URL.Path, wantPath)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer "+fixtureSecret {
			t.Errorf("Authorization = %q", got)
		}
		if got := r.Header.Get("Accept"); got != "application/json" {
			t.Errorf("Accept = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
}

func trafficClientForBody(t *testing.T, body string, limit int64) (*clashapi.Client, func()) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, body)
	}))
	client := newFixtureClient(t, server.URL, server.Client(), time.Second, limit)
	return client, server.Close
}
