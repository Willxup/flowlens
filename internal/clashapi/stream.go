package clashapi

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sync"
	"time"
)

const streamInitialBufferSize = 64 * 1024

// TrafficStream reads bounded newline-delimited traffic samples.
type TrafficStream struct {
	stream *jsonStream
}

// MemoryStream reads bounded newline-delimited memory samples.
type MemoryStream struct {
	stream *jsonStream
}

// Traffic opens the long-lived /traffic HTTP stream.
func (c *Client) Traffic(ctx context.Context) (*TrafficStream, error) {
	stream, err := c.openStream(ctx, "/traffic")
	if err != nil {
		return nil, err
	}
	return &TrafficStream{stream: stream}, nil
}

// Memory opens the optional long-lived /memory HTTP stream.
func (c *Client) Memory(ctx context.Context) (*MemoryStream, error) {
	stream, err := c.openStream(ctx, "/memory")
	if err != nil {
		return nil, err
	}
	return &MemoryStream{stream: stream}, nil
}

// Next returns the next non-empty traffic sample.
func (s *TrafficStream) Next() (TrafficSample, error) {
	token, err := s.stream.nextToken()
	if err != nil {
		return TrafficSample{}, err
	}
	var response struct {
		Up   *int64 `json:"up"`
		Down *int64 `json:"down"`
	}
	if err := json.Unmarshal(token, &response); err != nil {
		return TrafficSample{}, errors.New("Clash API traffic stream returned invalid JSON")
	}
	if response.Up == nil || response.Down == nil {
		return TrafficSample{}, errors.New("Clash API traffic stream is missing sample fields")
	}
	sample := TrafficSample{Up: *response.Up, Down: *response.Down}
	if sample.Up < 0 || sample.Down < 0 {
		return TrafficSample{}, errors.New("Clash API traffic stream returned a negative sample")
	}
	return sample, nil
}

// Close cancels and closes the traffic stream. It is safe to call repeatedly.
func (s *TrafficStream) Close() error {
	return s.stream.close()
}

// Next returns the next non-empty memory sample.
func (s *MemoryStream) Next() (MemorySample, error) {
	token, err := s.stream.nextToken()
	if err != nil {
		return MemorySample{}, err
	}
	var response struct {
		Inuse   *int64 `json:"inuse"`
		OSLimit *int64 `json:"oslimit"`
	}
	if err := json.Unmarshal(token, &response); err != nil {
		return MemorySample{}, errors.New("Clash API memory stream returned invalid JSON")
	}
	if response.Inuse == nil {
		return MemorySample{}, errors.New("Clash API memory stream is missing inuse")
	}
	sample := MemorySample{Inuse: *response.Inuse}
	if response.OSLimit != nil {
		sample.OSLimit = *response.OSLimit
	}
	if sample.Inuse < 0 || sample.OSLimit < 0 {
		return MemorySample{}, errors.New("Clash API memory stream returned a negative sample")
	}
	return sample, nil
}

// Close cancels and closes the memory stream. It is safe to call repeatedly.
func (s *MemoryStream) Close() error {
	return s.stream.close()
}

type jsonStream struct {
	scanner  *bufio.Scanner
	body     io.ReadCloser
	cancel   context.CancelFunc
	maxToken int64

	closeOnce sync.Once
	closeErr  error
}

func (c *Client) openStream(ctx context.Context, path string) (*jsonStream, error) {
	requestContext, cancel := context.WithCancel(ctx)
	request, err := http.NewRequestWithContext(requestContext, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		cancel()
		return nil, errors.New("cannot create Clash API stream request")
	}
	request.Header.Set("Authorization", "Bearer "+c.secret)
	request.Header.Set("Accept", "application/json")

	openingTimer := time.AfterFunc(c.requestTimeout, cancel)
	response, err := c.httpClient.Do(request)
	openingTimer.Stop()
	if err != nil {
		cancel()
		return nil, errors.New("Clash API stream request failed")
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		cancel()
		_ = response.Body.Close()
		return nil, errors.New("Clash API stream returned a non-success status")
	}

	initialSize := c.maxResponseSize + 1
	if initialSize > streamInitialBufferSize {
		initialSize = streamInitialBufferSize
	}
	scanner := bufio.NewScanner(response.Body)
	scanner.Buffer(make([]byte, int(initialSize)), int(c.maxResponseSize+1))
	return &jsonStream{
		scanner:  scanner,
		body:     response.Body,
		cancel:   cancel,
		maxToken: c.maxResponseSize,
	}, nil
}

func (s *jsonStream) nextToken() ([]byte, error) {
	for s.scanner.Scan() {
		token := bytes.TrimSpace(s.scanner.Bytes())
		if len(token) == 0 {
			continue
		}
		if int64(len(token)) > s.maxToken {
			return nil, errors.New("Clash API stream token exceeds configured limit")
		}
		if token[0] != '{' {
			return nil, errors.New("Clash API stream token is not a JSON object")
		}
		return token, nil
	}
	if s.scanner.Err() != nil {
		return nil, errors.New("cannot read Clash API stream")
	}
	return nil, io.EOF
}

func (s *jsonStream) close() error {
	s.closeOnce.Do(func() {
		s.cancel()
		if err := s.body.Close(); err != nil {
			s.closeErr = errors.New("cannot close Clash API stream")
		}
	})
	return s.closeErr
}
