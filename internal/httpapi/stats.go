package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"strconv"

	"github.com/Willxup/flowlens/internal/query"
	"github.com/Willxup/flowlens/internal/rollup"
)

// StatisticsQueries is the exact Stage 2 read-model boundary.
type StatisticsQueries interface {
	Overview(context.Context, rollup.Range) (query.Overview, error)
	Series(context.Context, rollup.Range) (query.Series, error)
	Quality(context.Context, rollup.Range) (query.Quality, error)
	Storage(context.Context) (query.Storage, error)
	Breakdown(context.Context, rollup.Range, query.BreakdownBy) (query.Breakdown, error)
	LiveTargets(context.Context) (query.LiveTargets, error)
	RuntimeSessions(context.Context) ([]query.RuntimeSessionRecord, error)
	Labels(context.Context) ([]query.Label, error)
	LabelCandidates(context.Context) ([]query.LabelCandidate, error)
	CreateLabel(context.Context, query.CreateLabel) (query.Label, error)
	UpdateLabel(context.Context, int64, string) (query.Label, error)
	DeleteLabel(context.Context, int64) (bool, error)
}

type totalsResponse struct {
	UploadBytes     string `json:"upload_bytes"`
	DownloadBytes   string `json:"download_bytes"`
	TotalBytes      string `json:"total_bytes"`
	ElapsedSeconds  int64  `json:"elapsed_seconds"`
	ObservedSeconds int64  `json:"observed_seconds"`
}

type overviewResponse struct {
	Current             totalsResponse `json:"current"`
	Previous            totalsResponse `json:"previous"`
	BoundaryApproximate bool           `json:"boundary_approximate"`
}

type seriesResponse struct {
	BoundaryApproximate bool                  `json:"boundary_approximate"`
	Points              []seriesPointResponse `json:"points"`
}

type seriesPointResponse struct {
	BucketStart                   int64  `json:"bucket_start"`
	BucketEnd                     int64  `json:"bucket_end"`
	ElapsedSeconds                int64  `json:"elapsed_seconds"`
	SourceResolutionSec           int64  `json:"source_resolution_sec"`
	UploadBytes                   string `json:"upload_bytes"`
	DownloadBytes                 string `json:"download_bytes"`
	RecoveredUploadBytes          string `json:"recovered_upload_bytes"`
	RecoveredDownloadBytes        string `json:"recovered_download_bytes"`
	UnattributedUploadBytes       string `json:"unattributed_upload_bytes"`
	UnattributedDownloadBytes     string `json:"unattributed_download_bytes"`
	AverageUploadBytesPerSecond   int64  `json:"average_upload_bytes_per_second"`
	AverageDownloadBytesPerSecond int64  `json:"average_download_bytes_per_second"`
	SpeedUploadSampleSum          string `json:"speed_upload_sample_sum"`
	SpeedDownloadSampleSum        string `json:"speed_download_sample_sum"`
	SpeedSampleCount              int64  `json:"speed_sample_count"`
	PeakUploadBytesPerSecond      int64  `json:"peak_upload_bytes_per_second"`
	PeakDownloadBytesPerSecond    int64  `json:"peak_download_bytes_per_second"`
	CounterObservedSeconds        int64  `json:"counter_observed_seconds"`
	ActiveConnectionsSum          int64  `json:"active_connections_sum"`
	ActiveConnectionsSamples      int64  `json:"active_connections_samples"`
	ActiveConnectionsMax          int64  `json:"active_connections_max"`
	ResetCount                    int64  `json:"reset_count"`
	QualityFlags                  int64  `json:"quality_flags"`
}

type qualityResponse struct {
	Events []qualityEventResponse `json:"events"`
}

type qualityEventResponse struct {
	Code      string `json:"code"`
	StartedAt int64  `json:"started_at"`
	EndedAt   *int64 `json:"ended_at"`
	Flags     int64  `json:"flags"`
}

type storageResponse struct {
	DatabaseBytes     string               `json:"database_bytes"`
	WALBytes          string               `json:"wal_bytes"`
	SoftLimitBytes    string               `json:"soft_limit_bytes"`
	Protecting        bool                 `json:"protecting"`
	LastRollupCleanup *maintenanceResponse `json:"last_rollup_cleanup"`
}

type maintenanceResponse struct {
	StartedAt   int64  `json:"started_at"`
	EndedAt     *int64 `json:"ended_at"`
	DeletedRows int64  `json:"deleted_rows"`
	Successful  bool   `json:"successful"`
}

func (h *handler) overviewResponse(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		writer.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	rangeValue, ok := strictRange(request, false)
	if !ok {
		writer.WriteHeader(http.StatusBadRequest)
		return
	}
	value, err := h.queries.Overview(request.Context(), rangeValue)
	if err != nil {
		writer.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	current, err := totalsDTO(value.Current)
	if err != nil {
		writer.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	previous, err := totalsDTO(value.Previous)
	if err != nil {
		writer.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	writeJSON(writer, overviewResponse{
		Current: current, Previous: previous, BoundaryApproximate: value.BoundaryApproximate,
	})
}

func (h *handler) seriesResponse(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		writer.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	rangeValue, ok := strictRange(request, true)
	if !ok {
		writer.WriteHeader(http.StatusBadRequest)
		return
	}
	value, err := h.queries.Series(request.Context(), rangeValue)
	if err != nil {
		writer.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	response := seriesResponse{BoundaryApproximate: value.BoundaryApproximate}
	response.Points = make([]seriesPointResponse, 0, len(value.Points))
	for _, point := range value.Points {
		averageUpload, averageDownload := int64(0), int64(0)
		if point.SpeedSampleCount > 0 {
			averageUpload = point.SpeedUploadSampleSum / point.SpeedSampleCount
			averageDownload = point.SpeedDownloadSampleSum / point.SpeedSampleCount
		}
		response.Points = append(response.Points, seriesPointResponse{
			BucketStart: point.BucketStart, BucketEnd: point.BucketEnd,
			ElapsedSeconds: point.BucketEnd - point.BucketStart, SourceResolutionSec: point.ResolutionSec,
			UploadBytes: strconv.FormatInt(point.UploadBytes, 10), DownloadBytes: strconv.FormatInt(point.DownloadBytes, 10),
			RecoveredUploadBytes:        strconv.FormatInt(point.RecoveredUploadBytes, 10),
			RecoveredDownloadBytes:      strconv.FormatInt(point.RecoveredDownloadBytes, 10),
			UnattributedUploadBytes:     strconv.FormatInt(point.UnattributedUploadBytes, 10),
			UnattributedDownloadBytes:   strconv.FormatInt(point.UnattributedDownloadBytes, 10),
			AverageUploadBytesPerSecond: averageUpload, AverageDownloadBytesPerSecond: averageDownload,
			SpeedUploadSampleSum:       strconv.FormatInt(point.SpeedUploadSampleSum, 10),
			SpeedDownloadSampleSum:     strconv.FormatInt(point.SpeedDownloadSampleSum, 10),
			SpeedSampleCount:           point.SpeedSampleCount,
			PeakUploadBytesPerSecond:   point.PeakUploadBytesPerSecond,
			PeakDownloadBytesPerSecond: point.PeakDownloadBytesPerSecond,
			CounterObservedSeconds:     point.CounterObservedSeconds,
			ActiveConnectionsSum:       point.ActiveConnectionsSum,
			ActiveConnectionsSamples:   point.ActiveConnectionsSamples,
			ActiveConnectionsMax:       point.ActiveConnectionsMax,
			ResetCount:                 point.ResetCount, QualityFlags: point.QualityFlags,
		})
	}
	writeJSON(writer, response)
}

func (h *handler) qualityResponse(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		writer.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	rangeValue, ok := strictRange(request, false)
	if !ok {
		writer.WriteHeader(http.StatusBadRequest)
		return
	}
	value, err := h.queries.Quality(request.Context(), rangeValue)
	if err != nil {
		writer.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	response := qualityResponse{Events: make([]qualityEventResponse, 0, len(value.Events))}
	for _, event := range value.Events {
		response.Events = append(response.Events, qualityEventResponse{
			Code: event.Code, StartedAt: event.StartedAt, EndedAt: event.EndedAt, Flags: event.Flags,
		})
	}
	writeJSON(writer, response)
}

func (h *handler) storageResponse(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		writer.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if len(request.URL.Query()) != 0 {
		writer.WriteHeader(http.StatusBadRequest)
		return
	}
	value, err := h.queries.Storage(request.Context())
	if err != nil {
		writer.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	response := storageResponse{
		DatabaseBytes:  strconv.FormatInt(value.Capacity.DatabaseBytes, 10),
		WALBytes:       strconv.FormatInt(value.Capacity.WALBytes, 10),
		SoftLimitBytes: strconv.FormatInt(value.Capacity.SoftLimitBytes, 10),
		Protecting:     value.Capacity.Protecting,
	}
	if value.LastRollupCleanup != nil {
		response.LastRollupCleanup = &maintenanceResponse{
			StartedAt:   value.LastRollupCleanup.StartedAt,
			EndedAt:     value.LastRollupCleanup.EndedAt,
			DeletedRows: value.LastRollupCleanup.DeletedRows,
			Successful:  value.LastRollupCleanup.Error == nil,
		}
	}
	writeJSON(writer, response)
}

func strictRange(request *http.Request, allowResolution bool) (rollup.Range, bool) {
	values := request.URL.Query()
	allowed := map[string]bool{"from": true, "to": true}
	if allowResolution {
		allowed["resolution"] = true
	}
	for key, entries := range values {
		if !allowed[key] || len(entries) != 1 {
			return rollup.Range{}, false
		}
	}
	if len(values["from"]) != 1 || len(values["to"]) != 1 {
		return rollup.Range{}, false
	}
	from, err := strconv.ParseInt(values.Get("from"), 10, 64)
	if err != nil || from <= 0 {
		return rollup.Range{}, false
	}
	to, err := strconv.ParseInt(values.Get("to"), 10, 64)
	if err != nil || to <= from {
		return rollup.Range{}, false
	}
	if resolution := values.Get("resolution"); resolution != "" && resolution != "auto" {
		return rollup.Range{}, false
	}
	return rollup.Range{From: from, To: to}, true
}

func totalsDTO(value query.Totals) (totalsResponse, error) {
	if value.UploadBytes < 0 || value.DownloadBytes < 0 || value.ElapsedSeconds < 0 || value.ObservedSeconds < 0 ||
		value.UploadBytes > math.MaxInt64-value.DownloadBytes {
		return totalsResponse{}, errors.New("invalid totals")
	}
	return totalsResponse{
		UploadBytes:    strconv.FormatInt(value.UploadBytes, 10),
		DownloadBytes:  strconv.FormatInt(value.DownloadBytes, 10),
		TotalBytes:     strconv.FormatInt(value.UploadBytes+value.DownloadBytes, 10),
		ElapsedSeconds: value.ElapsedSeconds, ObservedSeconds: value.ObservedSeconds,
	}, nil
}

func writeJSON(writer http.ResponseWriter, value any) {
	writeJSONStatus(writer, http.StatusOK, value)
}

func writeJSONStatus(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}
