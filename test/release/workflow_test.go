package release_test

import (
	"reflect"
	"strings"
	"testing"
)

func TestWorkflowsUseNode24ActionReleases(t *testing.T) {
	tests := []struct {
		name string
		path string
		want []string
	}{
		{
			name: "CI",
			path: ".github/workflows/ci.yml",
			want: []string{
				"actions/checkout@v5",
				"actions/setup-go@v6",
				"actions/setup-node@v5",
				"pnpm/action-setup@v5",
				"gitleaks/gitleaks-action@v3",
			},
		},
		{
			name: "release",
			path: ".github/workflows/release.yml",
			want: []string{
				"actions/checkout@v5",
				"docker/setup-buildx-action@v4",
				"docker/login-action@v4",
				"docker/metadata-action@v6",
				"docker/build-push-action@v7",
				"actions/upload-artifact@v6",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			contents := readRepositoryFile(t, test.path)
			if got := workflowActionReferences(contents); !reflect.DeepEqual(got, test.want) {
				t.Errorf("workflow action references = %#v, want Node.js 24 releases %#v", got, test.want)
			}
		})
	}
}

func workflowActionReferences(contents string) []string {
	var references []string
	for _, line := range strings.Split(contents, "\n") {
		line = strings.TrimSpace(line)
		line = strings.TrimPrefix(line, "- ")
		if reference, found := strings.CutPrefix(line, "uses: "); found {
			references = append(references, reference)
		}
	}
	return references
}

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
	want := "- name: Install Chromium\n        env:\n          PLAYWRIGHT_BROWSERS_PATH: ${{ github.workspace }}/.flowlens-dev/cache/playwright\n        run: pnpm --dir web exec playwright install --with-deps chromium"
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

func TestReleaseWorkflowPublishesOriginalVersionTag(t *testing.T) {
	contents := readRepositoryFile(t, ".github/workflows/release.yml")
	if !strings.Contains(contents, "type=raw,value=${{ github.ref_name }}") {
		t.Error("release workflow must preserve the original v-prefixed Git tag")
	}
}

func assertNoDeploymentPermissions(t *testing.T, name, contents string) {
	t.Helper()
	for _, forbidden := range []string{"pages:", "deployments:", "id-token:"} {
		if strings.Contains(contents, forbidden) {
			t.Errorf("%s workflow requests %q", name, forbidden)
		}
	}
}
