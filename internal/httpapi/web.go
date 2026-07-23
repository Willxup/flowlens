package httpapi

import (
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strconv"
	"strings"
)

var protectedPages = map[string]bool{
	"/": true, "/traffic": true, "/targets": true, "/storage": true,
}

func (h *handler) webResponse(writer http.ResponseWriter, request *http.Request) bool {
	name, page, ok := webResource(request.URL.Path)
	if !ok {
		return false
	}
	if request.Method != http.MethodGet && request.Method != http.MethodHead {
		writer.WriteHeader(http.StatusMethodNotAllowed)
		return true
	}
	if page && request.URL.Path == "/login" && h.authDisabled {
		http.Redirect(writer, request, "/", http.StatusFound)
		return true
	}
	if page && request.URL.Path != "/login" && !h.validSession(request) {
		http.Redirect(writer, request, "/login", http.StatusFound)
		return true
	}
	if h.web == nil || !fs.ValidPath(name) {
		writer.WriteHeader(http.StatusNotFound)
		return true
	}
	info, err := fs.Stat(h.web, name)
	if err != nil || info.IsDir() {
		writer.WriteHeader(http.StatusNotFound)
		return true
	}
	content, err := fs.ReadFile(h.web, name)
	if err != nil {
		writer.WriteHeader(http.StatusNotFound)
		return true
	}
	setWebSecurityHeaders(writer.Header())
	writer.Header().Set("Content-Type", webContentType(name))
	writer.Header().Set("Content-Length", strconv.Itoa(len(content)))
	if page || name == "theme-init.js" {
		writer.Header().Set("Cache-Control", "no-store")
	} else if strings.HasPrefix(name, "assets/") {
		writer.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		writer.Header().Set("Cache-Control", "public, max-age=3600")
	}
	writer.WriteHeader(http.StatusOK)
	if request.Method == http.MethodGet {
		_, _ = writer.Write(content)
	}
	return true
}

func webResource(urlPath string) (name string, page bool, ok bool) {
	if urlPath == "/login" || protectedPages[urlPath] {
		return "index.html", true, true
	}
	if urlPath == "/theme-init.js" || urlPath == "/favicon.svg" {
		return strings.TrimPrefix(urlPath, "/"), false, true
	}
	if strings.HasPrefix(urlPath, "/assets/") {
		name = strings.TrimPrefix(urlPath, "/")
		return name, false, fs.ValidPath(name)
	}
	return "", false, false
}

func webContentType(name string) string {
	switch path.Ext(name) {
	case ".html":
		return "text/html; charset=utf-8"
	case ".js":
		return "text/javascript; charset=utf-8"
	case ".css":
		return "text/css; charset=utf-8"
	case ".svg":
		return "image/svg+xml"
	default:
		if value := mime.TypeByExtension(path.Ext(name)); value != "" {
			return value
		}
		return "application/octet-stream"
	}
}
