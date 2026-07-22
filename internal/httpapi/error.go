package httpapi

import (
	"encoding/json"
	"net/http"
)

type apiResponseWriter struct {
	http.ResponseWriter
	wroteHeader bool
}

func (writer *apiResponseWriter) WriteHeader(status int) {
	if writer.wroteHeader {
		return
	}
	writer.wroteHeader = true
	if status < http.StatusBadRequest {
		writer.ResponseWriter.WriteHeader(status)
		return
	}
	writer.Header().Set("Content-Type", "application/json")
	writer.ResponseWriter.WriteHeader(status)
	payload, _ := json.Marshal(struct {
		Error string `json:"error"`
	}{Error: apiErrorCode(status)})
	_, _ = writer.ResponseWriter.Write(append(payload, '\n'))
}

func (writer *apiResponseWriter) Write(contents []byte) (int, error) {
	if !writer.wroteHeader {
		writer.WriteHeader(http.StatusOK)
	}
	return writer.ResponseWriter.Write(contents)
}

func (writer *apiResponseWriter) Unwrap() http.ResponseWriter {
	return writer.ResponseWriter
}

func apiErrorCode(status int) string {
	switch status {
	case http.StatusBadRequest:
		return "bad_request"
	case http.StatusUnauthorized:
		return "unauthorized"
	case http.StatusForbidden:
		return "forbidden"
	case http.StatusNotFound:
		return "not_found"
	case http.StatusMethodNotAllowed:
		return "method_not_allowed"
	case http.StatusConflict:
		return "conflict"
	case http.StatusRequestEntityTooLarge:
		return "request_too_large"
	case http.StatusUnsupportedMediaType:
		return "unsupported_media_type"
	case http.StatusServiceUnavailable:
		return "service_unavailable"
	default:
		return "request_failed"
	}
}
