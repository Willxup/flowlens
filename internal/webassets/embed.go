// Package webassets owns the immutable embedded production UI build.
package webassets

import (
	"embed"
	"io/fs"
)

//go:embed dist
var content embed.FS

// Content returns the build root without exposing the package directory prefix.
func Content() (fs.FS, error) {
	return fs.Sub(content, "dist")
}
