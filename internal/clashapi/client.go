package clashapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// Options configures a Clash API client.
type Options struct {
	BaseURL         string
	Secret          string
	RequestTimeout  time.Duration
	MaxResponseSize int64
	HTTPClient      *http.Client
}

// String prevents accidental disclosure when client options are logged.
func (Options) String() string {
	return "ClashAPIOptions{redacted}"
}

// GoString prevents configuration disclosure through Go-syntax formatting.
func (options Options) GoString() string {
	return options.String()
}

// Client performs bounded authenticated requests to one Clash API.
type Client struct {
	baseURL         string
	secret          string
	requestTimeout  time.Duration
	maxResponseSize int64
	httpClient      *http.Client
}

// String prevents accidental disclosure when a configured client is logged.
func (c *Client) String() string {
	return "ClashAPIClient{redacted}"
}

// GoString prevents configuration disclosure through Go-syntax formatting.
func (c *Client) GoString() string {
	return c.String()
}

// New validates options and constructs a client without mutating an injected
// HTTP client.
func New(options Options) (*Client, error) {
	baseURL, err := canonicalBaseURL(options.BaseURL)
	if err != nil {
		return nil, err
	}
	if options.Secret == "" {
		return nil, errors.New("Clash API secret is required")
	}
	if options.RequestTimeout <= 0 {
		return nil, errors.New("Clash API request timeout must be positive")
	}
	if options.MaxResponseSize <= 0 {
		return nil, errors.New("Clash API response limit must be positive")
	}
	maxInt := int64(^uint(0) >> 1)
	if options.MaxResponseSize >= maxInt {
		return nil, errors.New("Clash API response limit is too large")
	}

	httpClient := options.HTTPClient
	if httpClient == nil {
		transport, ok := http.DefaultTransport.(*http.Transport)
		if !ok {
			return nil, errors.New("default HTTP transport is unavailable")
		}
		clonedTransport := transport.Clone()
		clonedTransport.ResponseHeaderTimeout = options.RequestTimeout
		httpClient = &http.Client{Transport: clonedTransport}
	}
	clientCopy := *httpClient
	clientCopy.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	httpClient = &clientCopy

	return &Client{
		baseURL:         baseURL,
		secret:          options.Secret,
		requestTimeout:  options.RequestTimeout,
		maxResponseSize: options.MaxResponseSize,
		httpClient:      httpClient,
	}, nil
}

// Version retrieves the Clash implementation version.
func (c *Client) Version(ctx context.Context) (Version, error) {
	var version Version
	if err := c.getJSON(ctx, "/version", &version); err != nil {
		return Version{}, err
	}
	return version, nil
}

// Connections retrieves one finite cumulative counter and connection snapshot.
func (c *Client) Connections(ctx context.Context) (ConnectionsSnapshot, error) {
	var response struct {
		DownloadTotal *int64       `json:"downloadTotal"`
		UploadTotal   *int64       `json:"uploadTotal"`
		Connections   []Connection `json:"connections"`
		Memory        *int64       `json:"memory"`
	}
	if err := c.getJSON(ctx, "/connections", &response); err != nil {
		return ConnectionsSnapshot{}, err
	}
	if response.UploadTotal == nil || response.DownloadTotal == nil {
		return ConnectionsSnapshot{}, errors.New("Clash API connections response is missing global totals")
	}
	snapshot := ConnectionsSnapshot{
		UploadTotal:   *response.UploadTotal,
		DownloadTotal: *response.DownloadTotal,
		Connections:   response.Connections,
	}
	if response.Memory != nil {
		snapshot.Memory = *response.Memory
	}
	if snapshot.UploadTotal < 0 || snapshot.DownloadTotal < 0 || snapshot.Memory < 0 {
		return ConnectionsSnapshot{}, errors.New("Clash API connections response contains a negative total")
	}
	for _, connection := range snapshot.Connections {
		if connection.Upload < 0 || connection.Download < 0 {
			return ConnectionsSnapshot{}, errors.New("Clash API connection contains a negative total")
		}
	}
	return snapshot, nil
}

func (c *Client) getJSON(ctx context.Context, path string, destination any) error {
	requestContext, cancel := context.WithTimeout(ctx, c.requestTimeout)
	defer cancel()

	request, err := http.NewRequestWithContext(requestContext, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return errors.New("cannot create Clash API request")
	}
	request.Header.Set("Authorization", "Bearer "+c.secret)
	request.Header.Set("Accept", "application/json")

	response, err := c.httpClient.Do(request)
	if err != nil {
		return errors.New("Clash API request failed")
	}
	defer response.Body.Close()

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("Clash API returned HTTP status %d", response.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(response.Body, c.maxResponseSize+1))
	if err != nil {
		return errors.New("cannot read Clash API response")
	}
	if int64(len(body)) > c.maxResponseSize {
		return errors.New("Clash API response exceeds configured limit")
	}

	decoder := json.NewDecoder(bytes.NewReader(body))
	if err := decoder.Decode(destination); err != nil {
		return errors.New("Clash API returned invalid JSON")
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return errors.New("Clash API returned trailing JSON data")
	}
	return nil
}

func canonicalBaseURL(rawURL string) (string, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", errors.New("Clash API URL is invalid")
	}
	if parsed.Scheme != "http" || parsed.Opaque != "" || parsed.Hostname() == "" || parsed.Port() == "" {
		return "", errors.New("Clash API URL must be an HTTP root with an explicit port")
	}
	port, err := strconv.Atoi(parsed.Port())
	if err != nil || port < 1 || port > 65535 {
		return "", errors.New("Clash API URL contains an invalid port")
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", errors.New("Clash API URL must not contain user info, query, or fragment")
	}
	if parsed.Path != "" && parsed.Path != "/" {
		return "", errors.New("Clash API URL must not contain a path")
	}
	parsed.Path = ""
	parsed.RawPath = ""
	return parsed.String(), nil
}
