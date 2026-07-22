package release_test

import (
	"strings"
	"testing"
)

func TestCIWorkflowRunsProductAndReleaseChecksWithoutDeploymentPermissions(t *testing.T) {
	contents := readRepositoryFile(t, ".github/workflows/ci.yml")
	for _, value := range []string{
		"pull_request:",
		"branches: [main]",
		"go-version: \"1.26.2\"",
		"node-version: \"24.14.0\"",
		"version: 11.9.0",
		"make check",
		"make frontend-e2e",
		"CGO_ENABLED=0",
		"go mod verify",
		"gitleaks/gitleaks-action",
	} {
		if !strings.Contains(contents, value) {
			t.Errorf("CI workflow missing %q", value)
		}
	}
	for _, step := range strings.Split(contents, "\n      - ") {
		if strings.Contains(step, "uses: actions/setup-node@") && strings.Contains(step, "cache: false") {
			t.Error("setup-node cache input must be omitted or name npm, yarn, or pnpm")
		}
	}
	assertNoDeploymentPermissions(t, "CI", contents)
}

func TestCIInstallsPlaywrightBrowsersInTheProjectCache(t *testing.T) {
	contents := readRepositoryFile(t, ".github/workflows/ci.yml")
	want := "- name: Install Chromium\n        env:\n          PLAYWRIGHT_BROWSERS_PATH: .flowlens-dev/cache/playwright\n        run: pnpm --dir web exec playwright install --with-deps chromium"
	if !strings.Contains(contents, want) {
		t.Error("CI must install Playwright browsers in the same project-local cache used by Makefile")
	}
}

func TestReleaseWorkflowIsTagOnlyMultiArchitectureGHCRWithSBOM(t *testing.T) {
	contents := readRepositoryFile(t, ".github/workflows/release.yml")
	for _, value := range []string{
		"tags:",
		"- \"v*\"",
		"packages: write",
		"docker/metadata-action",
		"docker/setup-buildx-action",
		"docker/build-push-action",
		"ghcr.io/willxup/flowlens",
		"platforms: linux/amd64,linux/arm64",
		"push: true",
		"sbom: true",
		"provenance: false",
		"steps.build.outputs.digest",
		"actions/upload-artifact",
	} {
		if !strings.Contains(contents, value) {
			t.Errorf("release workflow missing %q", value)
		}
	}
	if strings.Contains(contents, "branches:") {
		t.Error("release workflow is branch-triggered")
	}
	assertNoDeploymentPermissions(t, "release", contents)
}

func assertNoDeploymentPermissions(t *testing.T, name, contents string) {
	t.Helper()
	for _, forbidden := range []string{"pages:", "deployments:", "id-token:"} {
		if strings.Contains(contents, forbidden) {
			t.Errorf("%s workflow requests %q", name, forbidden)
		}
	}
}
