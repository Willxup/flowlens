package httpapi

import (
	"mime"
	"net/url"
	"strings"
)

const maxLoginBodyBytes int64 = 4096

func setSecurityHeaders(headers mapHeader) {
	headers.Set("X-Content-Type-Options", "nosniff")
	headers.Set("Referrer-Policy", "no-referrer")
	headers.Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
}

type mapHeader interface {
	Set(string, string)
}

func sameOrigin(origin, host string) bool {
	if origin == "" || host == "" || strings.ContainsAny(host, " \t\r\n/@") {
		return false
	}
	parsed, err := url.Parse(origin)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.User != nil ||
		parsed.Host == "" || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return false
	}
	return strings.EqualFold(parsed.Host, host)
}

func isJSON(contentType string) bool {
	mediaType, _, err := mime.ParseMediaType(contentType)
	return err == nil && mediaType == "application/json"
}
