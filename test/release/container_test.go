package release_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

func TestDockerfileIsCrossCompiledAndCarriesReleaseMetadata(t *testing.T) {
	contents := readRepositoryFile(t, "Dockerfile")
	required := []string{
		"FROM --platform=$BUILDPLATFORM node:24.14.0-alpine AS web-build",
		"FROM --platform=$BUILDPLATFORM golang:1.26.2-alpine AS build",
		"ARG TARGETOS",
		"ARG TARGETARCH",
		"CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath",
		"github.com/Willxup/flowlens/internal/buildinfo.Version=$VERSION",
		"github.com/Willxup/flowlens/internal/buildinfo.Commit=$COMMIT",
		"github.com/Willxup/flowlens/internal/buildinfo.BuildDate=$BUILD_DATE",
		"org.opencontainers.image.source=\"https://github.com/Willxup/flowlens\"",
		"org.opencontainers.image.version=\"$VERSION\"",
		"org.opencontainers.image.revision=\"$COMMIT\"",
		"org.opencontainers.image.created=\"$BUILD_DATE\"",
		"FROM scratch",
		"USER 10001:10001",
		"HEALTHCHECK",
		"CMD [\"/flowlens\", \"healthcheck\"]",
	}
	for _, value := range required {
		if !strings.Contains(contents, value) {
			t.Errorf("Dockerfile missing %q", value)
		}
	}
}

func TestDockerfileDoesNotShadowBuildKitPlatformArguments(t *testing.T) {
	contents := readRepositoryFile(t, "Dockerfile")
	firstStage := strings.Index(contents, "\nFROM ")
	if firstStage < 0 {
		t.Fatal("Dockerfile has no build stage")
	}
	globalArguments := contents[:firstStage]
	for _, argument := range []string{"ARG BUILDPLATFORM", "ARG TARGETARCH"} {
		if hasLine(globalArguments, argument) {
			t.Errorf("Dockerfile shadows automatic BuildKit argument %q", argument)
		}
	}
}

func TestComposeUsesPublishedImageAndHardenedRuntime(t *testing.T) {
	contents := readRepositoryFile(t, "docker-compose.example.yml")
	var document map[string]any
	if err := yaml.Unmarshal([]byte(contents), &document); err != nil {
		t.Fatalf("compose YAML error = %v", err)
	}
	services := document["services"].(map[string]any)
	flowlens := services["flowlens"].(map[string]any)
	if _, exists := flowlens["build"]; exists {
		t.Error("compose still contains a source build")
	}
	if flowlens["image"] != "${FLOWLENS_IMAGE:-ghcr.io/willxup/flowlens:latest}" {
		t.Errorf("compose image = %#v", flowlens["image"])
	}
	if flowlens["user"] != "10001:10001" || flowlens["read_only"] != true {
		t.Errorf("compose identity/root = (%#v, %#v)", flowlens["user"], flowlens["read_only"])
	}
	assertStringList(t, flowlens, "tmpfs", []string{"/tmp:size=16m,mode=1777"})
	assertStringList(t, flowlens, "ports", []string{"127.0.0.1:8080:8080"})
	assertStringList(t, flowlens, "volumes", []string{
		"./config/config.yaml:/etc/flowlens/config.yaml:ro",
		"./data:/var/lib/flowlens",
	})
	assertStringList(t, flowlens, "security_opt", []string{"no-new-privileges:true"})
	assertStringList(t, flowlens, "cap_drop", []string{"ALL"})
	assertStringList(t, flowlens, "networks", []string{"flowlens_private"})
	if strings.Contains(contents, "9090:9090") {
		t.Error("compose publishes the Clash API port")
	}
}

func TestDockerContextExcludesLocalAndRuntimeData(t *testing.T) {
	contents := readRepositoryFile(t, ".dockerignore")
	for _, line := range []string{".git", ".flowlens-dev", "config/config.yaml", "data", "*.db", "*.db-wal", "*.db-shm"} {
		if !hasLine(contents, line) {
			t.Errorf(".dockerignore missing %q", line)
		}
	}
}

func TestFixtureDockerContextIncludesOnlyRequiredFiles(t *testing.T) {
	contents := readRepositoryFile(t, "test/release/fixture/Dockerfile.dockerignore")
	for _, line := range []string{
		"**",
		"!go.mod",
		"!go.sum",
		"!test/release/fixture/**",
		"!test/fixtures/clashapi/**",
	} {
		if !hasLine(contents, line) {
			t.Errorf("fixture Dockerfile.dockerignore missing %q", line)
		}
	}
}

func TestReleaseMakeTargetsDoNotPushByDefault(t *testing.T) {
	contents := readRepositoryFile(t, "Makefile")
	for _, value := range []string{
		"release-check:",
		"release-image:",
		"release-multiarch:",
		"PUSH ?= false",
		"ifeq ($(PUSH),true)",
		"--output type=oci,dest=",
		"--push",
	} {
		if !strings.Contains(contents, value) {
			t.Errorf("Makefile missing %q", value)
		}
	}
}

func readRepositoryFile(t *testing.T, name string) string {
	t.Helper()
	contents, err := os.ReadFile(filepath.Join("..", "..", name))
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", name, err)
	}
	return string(contents)
}

func assertStringList(t *testing.T, object map[string]any, key string, want []string) {
	t.Helper()
	raw, ok := object[key].([]any)
	if !ok {
		t.Fatalf("%s = %#v", key, object[key])
	}
	got := make([]string, len(raw))
	for index, value := range raw {
		got[index], ok = value.(string)
		if !ok {
			t.Fatalf("%s[%d] = %#v", key, index, value)
		}
	}
	if strings.Join(got, "\x00") != strings.Join(want, "\x00") {
		t.Errorf("%s = %#v, want %#v", key, got, want)
	}
}

func hasLine(contents, target string) bool {
	for _, line := range strings.Split(contents, "\n") {
		if strings.TrimSpace(line) == target {
			return true
		}
	}
	return false
}
