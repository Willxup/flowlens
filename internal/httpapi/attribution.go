package httpapi

import (
	"net/http"
	"strconv"

	"github.com/Willxup/flowlens/internal/query"
	"github.com/Willxup/flowlens/internal/rollup"
	"github.com/Willxup/flowlens/internal/storage"
)

type bytePairResponse struct {
	UploadBytes   string `json:"upload_bytes"`
	DownloadBytes string `json:"download_bytes"`
}

type breakdownItemResponse struct {
	RawValue      string `json:"raw_value"`
	DisplayName   string `json:"display_name"`
	NetworkCode   int64  `json:"network_code"`
	UploadBytes   string `json:"upload_bytes"`
	DownloadBytes string `json:"download_bytes"`
}

type breakdownResponseDTO struct {
	By                  query.BreakdownBy       `json:"by"`
	Available           bool                    `json:"available"`
	Approximate         bool                    `json:"approximate"`
	BoundaryApproximate bool                    `json:"boundary_approximate"`
	NoTraffic           bool                    `json:"no_traffic"`
	ConnectionCoverage  *float64                `json:"connection_coverage"`
	DimensionRetention  *float64                `json:"dimension_retention"`
	Global              bytePairResponse        `json:"global"`
	Other               bytePairResponse        `json:"other"`
	Unattributed        bytePairResponse        `json:"unattributed"`
	Items               []breakdownItemResponse `json:"items"`
}

type liveTargetResponse struct {
	RawEndpoint            string `json:"raw_endpoint"`
	DisplayName            string `json:"display_name"`
	NetworkCode            int64  `json:"network_code"`
	Host                   string `json:"host"`
	UploadBytesPerSecond   int64  `json:"upload_bytes_per_second"`
	DownloadBytesPerSecond int64  `json:"download_bytes_per_second"`
}

type runtimeSessionResponse struct {
	StartedAt            int64   `json:"started_at"`
	EndedAt              *int64  `json:"ended_at"`
	StartReason          string  `json:"start_reason"`
	EndReason            *string `json:"end_reason"`
	LastSeenAt           int64   `json:"last_seen_at"`
	SingBoxVersion       string  `json:"sing_box_version"`
	DataGapBeforeSeconds int64   `json:"data_gap_before_seconds"`
}

func (handler *handler) breakdownResponse(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		writer.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	rangeValue, by, ok := strictBreakdown(request)
	if !ok {
		writer.WriteHeader(http.StatusBadRequest)
		return
	}
	value, err := handler.queries.Breakdown(request.Context(), rangeValue, by)
	if err != nil {
		writer.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	response := breakdownResponseDTO{
		By: value.By, Available: value.Available, Approximate: true,
		BoundaryApproximate: value.BoundaryApproximate, NoTraffic: value.NoTraffic,
		ConnectionCoverage: value.ConnectionCoverage, DimensionRetention: value.DimensionRetention,
		Global: bytePairDTO(value.Global), Other: bytePairDTO(value.Other),
		Unattributed: bytePairDTO(value.Unattributed), Items: make([]breakdownItemResponse, 0, len(value.Items)),
	}
	for _, item := range value.Items {
		response.Items = append(response.Items, breakdownItemResponse{
			RawValue: item.RawValue, DisplayName: item.DisplayName, NetworkCode: item.NetworkCode,
			UploadBytes: strconv.FormatInt(item.UploadBytes, 10), DownloadBytes: strconv.FormatInt(item.DownloadBytes, 10),
		})
	}
	writeJSON(writer, response)
}

func (handler *handler) liveTargetsResponse(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		writer.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if len(request.URL.Query()) != 0 {
		writer.WriteHeader(http.StatusBadRequest)
		return
	}
	value, err := handler.queries.LiveTargets(request.Context())
	if err != nil {
		writer.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	targets := make([]liveTargetResponse, 0, len(value.Targets))
	for _, target := range value.Targets {
		targets = append(targets, liveTargetResponse{
			RawEndpoint: target.RawEndpoint, DisplayName: target.DisplayName,
			NetworkCode: target.NetworkCode, Host: target.Host,
			UploadBytesPerSecond:   target.UploadBytesPerSecond,
			DownloadBytesPerSecond: target.DownloadBytesPerSecond,
		})
	}
	writeJSON(writer, struct {
		ObservedAt         int64                `json:"observed_at"`
		IntervalMillis     int64                `json:"interval_millis"`
		ActiveConnections  int64                `json:"active_connections"`
		ConnectionCoverage *float64             `json:"connection_coverage"`
		Targets            []liveTargetResponse `json:"targets"`
	}{value.ObservedAt, value.IntervalMillis, value.ActiveConnections, value.ConnectionCoverage, targets})
}

func (handler *handler) runtimeSessionsResponse(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		writer.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if len(request.URL.Query()) != 0 {
		writer.WriteHeader(http.StatusBadRequest)
		return
	}
	value, err := handler.queries.RuntimeSessions(request.Context())
	if err != nil {
		writer.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	sessions := make([]runtimeSessionResponse, 0, len(value))
	for _, session := range value {
		sessions = append(sessions, runtimeSessionResponse{
			StartedAt: session.StartedAt, EndedAt: session.EndedAt,
			StartReason: session.StartReason, EndReason: session.EndReason,
			LastSeenAt: session.LastSeenAt, SingBoxVersion: session.SingBoxVersion,
			DataGapBeforeSeconds: session.DataGapBeforeSeconds,
		})
	}
	writeJSON(writer, struct {
		Sessions []runtimeSessionResponse `json:"sessions"`
	}{Sessions: sessions})
}

func strictBreakdown(request *http.Request) (rollup.Range, query.BreakdownBy, bool) {
	values := request.URL.Query()
	for key, entries := range values {
		if (key != "from" && key != "to" && key != "by") || len(entries) != 1 {
			return rollup.Range{}, "", false
		}
	}
	if len(values) != 3 {
		return rollup.Range{}, "", false
	}
	from, err := strconv.ParseInt(values.Get("from"), 10, 64)
	if err != nil || from <= 0 {
		return rollup.Range{}, "", false
	}
	to, err := strconv.ParseInt(values.Get("to"), 10, 64)
	if err != nil || to <= from {
		return rollup.Range{}, "", false
	}
	by := query.BreakdownBy(values.Get("by"))
	if by != query.ByTarget && by != query.ByEndpoint && by != query.ByPort && by != query.ByNetwork && by != query.BySource && by != query.ByDomain {
		return rollup.Range{}, "", false
	}
	return rollup.Range{From: from, To: to}, by, true
}

func bytePairDTO(value storage.ByteTotals) bytePairResponse {
	return bytePairResponse{UploadBytes: strconv.FormatInt(value.Upload, 10), DownloadBytes: strconv.FormatInt(value.Download, 10)}
}
