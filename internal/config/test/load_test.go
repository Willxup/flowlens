package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Willxup/flowlens/internal/config"
)

func TestLoadFixedPath(t *testing.T) {
	mode := os.Getenv("FLOWLENS_FIXED_CONFIG_TEST")
	if mode == "" {
		t.Skip("fixed-path behavior runs in a container-controlled filesystem")
	}

	cfg, err := config.Load()
	switch mode {
	case "valid":
		if err != nil {
			t.Fatalf("Load returned an error: %v", err)
		}
		if cfg.SchemaVersion != 1 {
			t.Fatalf("schema version = %d, want 1", cfg.SchemaVersion)
		}
	case "missing":
		assertLoadError(t, err, config.Path)
	case "nonregular":
		assertLoadError(t, err, "regular file")
	default:
		t.Fatalf("unknown FLOWLENS_FIXED_CONFIG_TEST mode %q", mode)
	}
}

func TestProductionConfigDoesNotReadEnvironment(t *testing.T) {
	paths, err := filepath.Glob("../*.go")
	if err != nil {
		t.Fatalf("glob production config sources: %v", err)
	}
	if len(paths) == 0 {
		t.Fatal("no production config sources found")
	}

	var source strings.Builder
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		source.Write(data)
	}

	for _, forbidden := range []string{"os.Getenv", "os.LookupEnv", "FLOWLENS_", "flag.String", "flag.Func"} {
		if strings.Contains(source.String(), forbidden) {
			t.Fatalf("production config source contains forbidden override mechanism %q", forbidden)
		}
	}
	if count := strings.Count(source.String(), config.Path); count != 1 {
		t.Fatalf("fixed config path occurs %d times in production source, want 1", count)
	}
}

func assertLoadError(t *testing.T, err error, expected string) {
	t.Helper()
	if err == nil {
		t.Fatal("Load unexpectedly succeeded")
	}
	if !strings.Contains(err.Error(), expected) {
		t.Fatalf("error %q does not contain %q", err, expected)
	}
	for _, secret := range []string{"请替换为真实 Clash API Secret", "请替换为至少16字符的随机登录密钥"} {
		if strings.Contains(err.Error(), secret) {
			t.Fatalf("error leaked secret %q", secret)
		}
	}
}
