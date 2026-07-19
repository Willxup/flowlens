package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/Willxup/flowlens/internal/query"
	"github.com/Willxup/flowlens/internal/storage"
)

const maxLabelBodyBytes int64 = 1024

type labelResponse struct {
	ID          int64  `json:"id"`
	LabelType   string `json:"label_type"`
	MatchValue  string `json:"match_value"`
	DisplayName string `json:"display_name"`
	CreatedAt   int64  `json:"created_at"`
	UpdatedAt   int64  `json:"updated_at"`
}

type labelCandidateResponse struct {
	LabelType     string `json:"label_type"`
	MatchValue    string `json:"match_value"`
	DisplayName   string `json:"display_name"`
	UploadBytes   string `json:"upload_bytes"`
	DownloadBytes string `json:"download_bytes"`
}

func (handler *handler) labelsResponse(writer http.ResponseWriter, request *http.Request) {
	switch request.Method {
	case http.MethodGet:
		if len(request.URL.Query()) != 0 {
			writer.WriteHeader(http.StatusBadRequest)
			return
		}
		values, err := handler.queries.Labels(request.Context())
		if err != nil {
			writer.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		labels := make([]labelResponse, 0, len(values))
		for _, value := range values {
			labels = append(labels, labelDTO(value))
		}
		writeJSON(writer, struct {
			Labels []labelResponse `json:"labels"`
		}{Labels: labels})
	case http.MethodPost:
		if !handler.validLabelWrite(writer, request) {
			return
		}
		var payload struct {
			LabelType   string `json:"label_type"`
			MatchValue  string `json:"match_value"`
			DisplayName string `json:"display_name"`
		}
		if !decodeStrictLabelJSON(writer, request, &payload) {
			return
		}
		input := query.CreateLabel{LabelType: payload.LabelType, MatchValue: payload.MatchValue, DisplayName: payload.DisplayName}
		value, err := handler.queries.CreateLabel(request.Context(), input)
		if err != nil {
			writeLabelError(writer, err)
			return
		}
		writeJSONStatus(writer, http.StatusCreated, labelDTO(value))
	default:
		writer.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (handler *handler) labelCandidatesResponse(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		writer.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if len(request.URL.Query()) != 0 {
		writer.WriteHeader(http.StatusBadRequest)
		return
	}
	values, err := handler.queries.LabelCandidates(request.Context())
	if err != nil {
		writer.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	candidates := make([]labelCandidateResponse, 0, len(values))
	for _, value := range values {
		candidates = append(candidates, labelCandidateResponse{
			LabelType: value.LabelType, MatchValue: value.MatchValue, DisplayName: value.DisplayName,
			UploadBytes: strconv.FormatInt(value.UploadBytes, 10), DownloadBytes: strconv.FormatInt(value.DownloadBytes, 10),
		})
	}
	writeJSON(writer, struct {
		Candidates []labelCandidateResponse `json:"candidates"`
	}{Candidates: candidates})
}

func (handler *handler) labelItemResponse(writer http.ResponseWriter, request *http.Request) {
	id, ok := strictLabelID(strings.TrimPrefix(request.URL.Path, "/api/v1/labels/"))
	if !ok || len(request.URL.Query()) != 0 {
		writer.WriteHeader(http.StatusBadRequest)
		return
	}
	switch request.Method {
	case http.MethodPut:
		if !handler.validLabelWrite(writer, request) {
			return
		}
		var payload struct {
			DisplayName string `json:"display_name"`
		}
		if !decodeStrictLabelJSON(writer, request, &payload) {
			return
		}
		value, err := handler.queries.UpdateLabel(request.Context(), id, payload.DisplayName)
		if err != nil {
			writeLabelError(writer, err)
			return
		}
		writeJSON(writer, labelDTO(value))
	case http.MethodDelete:
		if !sameOrigin(request.Header.Get("Origin"), request.Host) {
			writer.WriteHeader(http.StatusForbidden)
			return
		}
		deleted, err := handler.queries.DeleteLabel(request.Context(), id)
		if err != nil {
			writeLabelError(writer, err)
			return
		}
		if !deleted {
			writer.WriteHeader(http.StatusNotFound)
			return
		}
		writer.WriteHeader(http.StatusNoContent)
	default:
		writer.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (handler *handler) validLabelWrite(writer http.ResponseWriter, request *http.Request) bool {
	if !sameOrigin(request.Header.Get("Origin"), request.Host) {
		writer.WriteHeader(http.StatusForbidden)
		return false
	}
	if !isJSON(request.Header.Get("Content-Type")) {
		writer.WriteHeader(http.StatusUnsupportedMediaType)
		return false
	}
	return true
}

func decodeStrictLabelJSON(writer http.ResponseWriter, request *http.Request, target any) bool {
	request.Body = http.MaxBytesReader(writer, request.Body, maxLabelBodyBytes)
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		var sizeError *http.MaxBytesError
		if errors.As(err, &sizeError) {
			writer.WriteHeader(http.StatusRequestEntityTooLarge)
		} else {
			writer.WriteHeader(http.StatusBadRequest)
		}
		return false
	}
	if requireJSONEOF(decoder) != nil {
		writer.WriteHeader(http.StatusBadRequest)
		return false
	}
	return true
}

func strictLabelID(raw string) (int64, bool) {
	value, err := strconv.ParseInt(raw, 10, 64)
	return value, err == nil && value > 0 && strconv.FormatInt(value, 10) == raw
}

func writeLabelError(writer http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, storage.ErrInvalidLabel):
		writer.WriteHeader(http.StatusBadRequest)
	case errors.Is(err, storage.ErrLabelNotFound):
		writer.WriteHeader(http.StatusNotFound)
	case errors.Is(err, storage.ErrLabelConflict):
		writer.WriteHeader(http.StatusConflict)
	default:
		writer.WriteHeader(http.StatusServiceUnavailable)
	}
}

func labelDTO(value query.Label) labelResponse {
	return labelResponse{
		ID: value.ID, LabelType: value.LabelType, MatchValue: value.MatchValue,
		DisplayName: value.DisplayName, CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt,
	}
}
