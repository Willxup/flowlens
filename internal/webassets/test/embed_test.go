package webassets_test

import (
	"io/fs"
	"regexp"
	"strings"
	"testing"

	"github.com/Willxup/flowlens/internal/webassets"
)

func TestEmbeddedProductionBuildContainsReferencedAssets(t *testing.T) {
	content, err := webassets.Content()
	if err != nil {
		t.Fatalf("Content() error = %v", err)
	}
	index, err := fs.ReadFile(content, "index.html")
	if err != nil || len(index) < 100 {
		t.Fatalf("index.html = %d bytes, %v", len(index), err)
	}
	themeScript := strings.Index(string(index), `src="/theme-init.js"`)
	moduleScript := strings.Index(string(index), `type="module"`)
	if themeScript < 0 || moduleScript < 0 || themeScript > moduleScript {
		t.Fatalf("theme initializer must load before the application module: %s", index)
	}
	for _, name := range []string{"theme-init.js", "favicon.svg"} {
		value, err := fs.ReadFile(content, name)
		if err != nil || len(value) == 0 {
			t.Errorf("%s = %d bytes, %v", name, len(value), err)
		}
	}
	for _, match := range regexp.MustCompile(`(?:src|href)="/([^"?]+\.(?:js|css))"`).FindAllSubmatch(index, -1) {
		if _, err := fs.Stat(content, string(match[1])); err != nil {
			t.Errorf("index references missing %q: %v", match[1], err)
		}
	}
}
