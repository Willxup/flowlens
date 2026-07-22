package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/Willxup/flowlens/internal/collector"
	flowstatus "github.com/Willxup/flowlens/internal/status"
)

const (
	livePollInterval = time.Second
	liveHeartbeat    = 15 * time.Second
	liveWriteTimeout = 5 * time.Second
)

type liveSampleResponse struct {
	Timestamp              int64                  `json:"timestamp"`
	UploadBytesPerSecond   int64                  `json:"upload_bytes_per_second"`
	DownloadBytesPerSecond int64                  `json:"download_bytes_per_second"`
	ActiveConnections      int64                  `json:"active_connections"`
	Status                 collector.SampleStatus `json:"status"`
}

func (h *handler) liveResponse(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		writer.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if len(request.URL.Query()) != 0 {
		writer.WriteHeader(http.StatusBadRequest)
		return
	}
	if h.live == nil {
		writer.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	writer.Header().Set("Content-Type", "text/event-stream")
	writer.Header().Set("Cache-Control", "no-cache")
	writer.Header().Set("Connection", "keep-alive")
	writer.Header().Set("X-Accel-Buffering", "no")
	writer.WriteHeader(http.StatusOK)

	sequence := int64(1)
	samples := h.live.Snapshot()
	if err := writeLiveEvent(writer, sequence, "snapshot", struct {
		Sequence int64                `json:"sequence"`
		Samples  []liveSampleResponse `json:"samples"`
	}{Sequence: sequence, Samples: liveSampleDTOs(samples)}); err != nil {
		return
	}
	lastTimestamp := int64(0)
	if len(samples) > 0 {
		lastTimestamp = samples[len(samples)-1].Timestamp.UnixNano()
	}
	sequence++
	lastStatus := h.status.Snapshot()
	if err := writeStatusEvent(writer, sequence, lastStatus); err != nil {
		return
	}

	poll := time.NewTicker(livePollInterval)
	heartbeat := time.NewTicker(liveHeartbeat)
	defer poll.Stop()
	defer heartbeat.Stop()
	for {
		select {
		case <-request.Context().Done():
			return
		case <-poll.C:
			if !h.validSession(request) {
				return
			}
			for _, sample := range h.live.Snapshot() {
				stamp := sample.Timestamp.UnixNano()
				if stamp <= lastTimestamp {
					continue
				}
				sequence++
				if err := writeLiveEvent(writer, sequence, "sample", struct {
					Sequence int64              `json:"sequence"`
					Sample   liveSampleResponse `json:"sample"`
				}{Sequence: sequence, Sample: liveSampleDTO(sample)}); err != nil {
					return
				}
				lastTimestamp = stamp
			}
			status := h.status.Snapshot()
			if status != lastStatus {
				sequence++
				if err := writeStatusEvent(writer, sequence, status); err != nil {
					return
				}
				lastStatus = status
			}
		case now := <-heartbeat.C:
			sequence++
			if err := writeLiveEvent(writer, sequence, "heartbeat", struct {
				Sequence int64 `json:"sequence"`
				At       int64 `json:"at"`
			}{Sequence: sequence, At: now.UTC().Unix()}); err != nil {
				return
			}
		}
	}
}

func writeStatusEvent(writer http.ResponseWriter, sequence int64, status flowstatus.Snapshot) error {
	return writeLiveEvent(writer, sequence, "status", struct {
		Sequence int64            `json:"sequence"`
		Status   flowstatus.Level `json:"status"`
		Reason   string           `json:"reason"`
		Ready    bool             `json:"ready"`
	}{Sequence: sequence, Status: status.Level, Reason: status.Reason, Ready: status.Ready})
}

func writeLiveEvent(writer http.ResponseWriter, sequence int64, event string, value any) error {
	payload, err := json.Marshal(value)
	if err != nil {
		return err
	}
	controller := http.NewResponseController(writer)
	_ = controller.SetWriteDeadline(time.Now().Add(liveWriteTimeout))
	if _, err := fmt.Fprintf(writer, "id: %s\nevent: %s\ndata: %s\n\n", strconv.FormatInt(sequence, 10), event, payload); err != nil {
		return err
	}
	return controller.Flush()
}

func liveSampleDTOs(samples []collector.SpeedSample) []liveSampleResponse {
	result := make([]liveSampleResponse, 0, len(samples))
	for _, sample := range samples {
		result = append(result, liveSampleDTO(sample))
	}
	return result
}

func liveSampleDTO(sample collector.SpeedSample) liveSampleResponse {
	return liveSampleResponse{
		Timestamp:              sample.Timestamp.UTC().Unix(),
		UploadBytesPerSecond:   sample.UploadBytesPerSecond,
		DownloadBytesPerSecond: sample.DownloadBytesPerSecond,
		ActiveConnections:      sample.ActiveConnections,
		Status:                 sample.Status,
	}
}
