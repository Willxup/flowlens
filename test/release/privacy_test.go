package release_test

import (
	"bytes"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

var (
	ipv4Pattern       = regexp.MustCompile(`\b(?:[0-9]{1,3}\.){3}[0-9]{1,3}\b`)
	credentialPattern = regexp.MustCompile(`(?im)\b(?:secret|access_key)\b["']?\s*[:=]\s*["']?([A-Za-z0-9+/=_-]{16,})`)
)

func TestCredentialPattern(t *testing.T) {
	credential := "01234567" + "89abcdef"
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{name: "yaml secret", input: "secret: " + credential, want: true},
		{name: "unquoted assignment", input: "access_key=" + credential, want: true},
		{name: "json access key", input: `{"access_key": "` + credential + `"}`, want: true},
		{name: "compact json secret", input: `{"secret":"` + credential + `"}`, want: true},
		{name: "ordinary documentation", input: "Configure secret or access_key before deployment.", want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := credentialPattern.MatchString(test.input); got != test.want {
				t.Fatalf("credentialPattern.MatchString(%q) = %t, want %t", test.input, got, test.want)
			}
		})
	}
}

func TestReleaseTreeContainsNoPrivateOrRuntimeMaterial(t *testing.T) {
	files := releaseCandidateFiles(t)
	for _, path := range files {
		lower := strings.ToLower(path)
		if path == "config/config.yaml" || strings.HasSuffix(lower, ".db") ||
			strings.HasSuffix(lower, ".db-wal") || strings.HasSuffix(lower, ".db-shm") ||
			strings.HasSuffix(lower, ".db.zst") || strings.HasSuffix(lower, ".manifest.json") {
			t.Errorf("runtime file is present in the release tree: %s", path)
			continue
		}
		contents, err := os.ReadFile(filepath.Join("..", "..", path))
		if err != nil {
			t.Fatalf("ReadFile(%q) error = %v", path, err)
		}
		privateMarkers := [][]byte{
			[]byte("/" + "Users/"),
			[]byte("/srv/" + "flowlens"),
			[]byte("-----BEGIN " + "PRIVATE KEY-----"),
			[]byte("-----BEGIN RSA " + "PRIVATE KEY-----"),
			[]byte("-----BEGIN OPENSSH " + "PRIVATE KEY-----"),
			[]byte("gh" + "p_"),
			[]byte("github_" + "pat_"),
			[]byte("AK" + "IA"),
			[]byte("xox" + "b-"),
		}
		for _, marker := range privateMarkers {
			// Existing regression tests may quote a marker to assert its absence.
			contents = bytes.ReplaceAll(contents, append(append([]byte{'"'}, marker...), '"'), nil)
		}
		for _, forbidden := range privateMarkers {
			if bytes.Contains(contents, forbidden) {
				t.Errorf("%s contains forbidden private marker %q", path, forbidden)
			}
		}
		if strings.HasPrefix(path, "test/fixtures/") || strings.HasPrefix(path, "web/src/demo/") {
			assertDocumentationIPv4Only(t, path, contents)
		}
	}
}

func TestReleaseTreeContainsNoObviousLiveCredentials(t *testing.T) {
	allowed := [][]byte{
		[]byte("fixture-clash-secret"),
		[]byte("fixture-access-key-123456"),
		[]byte("fixture-access-key"),
		[]byte("environment-clash-secret"),
		[]byte("environment-access-key"),
	}
	for _, path := range releaseCandidateFiles(t) {
		contents, err := os.ReadFile(filepath.Join("..", "..", path))
		if err != nil {
			t.Fatalf("ReadFile(%q) error = %v", path, err)
		}
		for _, value := range allowed {
			contents = bytes.ReplaceAll(contents, value, nil)
		}
		if match := credentialPattern.FindSubmatch(contents); len(match) != 0 {
			t.Errorf("%s contains an obvious credential assignment", path)
		}
	}
}

func releaseCandidateFiles(t *testing.T) []string {
	t.Helper()
	command := exec.Command("git", "ls-files", "--cached", "--others", "--exclude-standard", "-z")
	command.Dir = filepath.Join("..", "..")
	output, err := command.Output()
	if err != nil {
		t.Fatalf("git ls-files error = %v", err)
	}
	parts := bytes.Split(output, []byte{0})
	files := make([]string, 0, len(parts))
	for _, part := range parts {
		if len(part) == 0 {
			continue
		}
		path := string(part)
		if _, err := os.Stat(filepath.Join("..", "..", path)); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			t.Fatalf("Stat(%q) error = %v", path, err)
		}
		files = append(files, path)
	}
	sort.Strings(files)
	return files
}

func assertDocumentationIPv4Only(t *testing.T, path string, contents []byte) {
	t.Helper()
	allowed := []*net.IPNet{
		mustCIDR(t, "192.0.2.0/24"),
		mustCIDR(t, "198.51.100.0/24"),
		mustCIDR(t, "203.0.113.0/24"),
	}
	for _, raw := range ipv4Pattern.FindAll(contents, -1) {
		address := net.ParseIP(string(raw))
		if address == nil {
			t.Errorf("%s contains malformed IPv4 fixture %q", path, raw)
			continue
		}
		valid := false
		for _, network := range allowed {
			valid = valid || network.Contains(address)
		}
		if !valid {
			t.Errorf("%s contains non-documentation IPv4 fixture %q", path, raw)
		}
	}
}

func mustCIDR(t *testing.T, value string) *net.IPNet {
	t.Helper()
	_, network, err := net.ParseCIDR(value)
	if err != nil {
		t.Fatalf("ParseCIDR(%q) error = %v", value, err)
	}
	return network
}
