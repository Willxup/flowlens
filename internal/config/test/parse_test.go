package config_test

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Willxup/flowlens/internal/config"
	"go.yaml.in/yaml/v3"
)

const examplePath = "../../../config/config.example.yaml"

func exampleYAML(t *testing.T) string {
	t.Helper()

	data, err := os.ReadFile(examplePath)
	if err != nil {
		t.Fatalf("read example config: %v", err)
	}
	return string(data)
}

func parseExample(t *testing.T) config.Config {
	t.Helper()

	cfg, err := config.Parse(strings.NewReader(exampleYAML(t)))
	if err != nil {
		t.Fatalf("parse example config: %v", err)
	}
	return cfg
}

func TestExampleConfigLoads(t *testing.T) {
	cfg := parseExample(t)

	if cfg.SchemaVersion != 1 {
		t.Fatalf("schema version = %d, want 1", cfg.SchemaVersion)
	}
	if cfg.Server.Listen != "0.0.0.0:8080" {
		t.Fatalf("server listen = %q", cfg.Server.Listen)
	}
	if cfg.ClashAPI.URL != "http://sing-box:9090" {
		t.Fatalf("clash api URL = %q", cfg.ClashAPI.URL)
	}
	if cfg.ClashAPI.Secret.Value() != "请替换为真实 Clash API Secret" {
		t.Fatal("clash api secret was not parsed exactly")
	}
	if cfg.ClashAPI.RequestTimeout.Duration != 3*time.Second {
		t.Fatalf("request timeout = %s", cfg.ClashAPI.RequestTimeout.Duration)
	}
	if cfg.ClashAPI.ConnectionsInterval.Duration != time.Second {
		t.Fatalf("connections interval = %s", cfg.ClashAPI.ConnectionsInterval.Duration)
	}
	if int64(cfg.ClashAPI.MaxResponseSize) != 16<<20 {
		t.Fatalf("max response size = %d", cfg.ClashAPI.MaxResponseSize)
	}
	if cfg.Auth.AccessKey.Value() != "请替换为至少16字符的随机登录密钥" {
		t.Fatal("access key was not parsed exactly")
	}
	if cfg.Auth.SessionTTL.Duration != 24*time.Hour {
		t.Fatalf("session TTL = %s", cfg.Auth.SessionTTL.Duration)
	}
	if int64(cfg.Storage.SoftLimit) != 256<<20 {
		t.Fatalf("soft limit = %d", cfg.Storage.SoftLimit)
	}
	if cfg.Time.Timezone != "Asia/Shanghai" {
		t.Fatalf("timezone = %q", cfg.Time.Timezone)
	}
	if cfg.Retention.TopK != 20 {
		t.Fatalf("top K = %d", cfg.Retention.TopK)
	}
	if cfg.Backup.LocalTime.Hour != 4 || cfg.Backup.LocalTime.Minute != 0 {
		t.Fatalf("backup local time = %02d:%02d", cfg.Backup.LocalTime.Hour, cfg.Backup.LocalTime.Minute)
	}
}

func TestParseRejectsUnknownField(t *testing.T) {
	input := exampleYAML(t) + "\nunexpected: true\n"
	if _, err := config.Parse(strings.NewReader(input)); err == nil {
		t.Fatal("Parse accepted an unknown field")
	}
}

func TestParseRejectsMultipleDocuments(t *testing.T) {
	input := exampleYAML(t) + "\n---\nschema_version: 1\n"
	if _, err := config.Parse(strings.NewReader(input)); err == nil {
		t.Fatal("Parse accepted multiple YAML documents")
	}
}

func TestParseRejectsMissingRequiredField(t *testing.T) {
	input := strings.Replace(exampleYAML(t), "schema_version: 1", "", 1)
	if _, err := config.Parse(strings.NewReader(input)); err == nil {
		t.Fatal("Parse accepted a missing schema_version")
	}
}

func TestParseDoesNotReadEnvironmentOverrides(t *testing.T) {
	t.Setenv("FLOWLENS_SERVER_LISTEN", "127.0.0.1:9999")
	t.Setenv("FLOWLENS_CLASH_API_SECRET", "environment-clash-secret")
	t.Setenv("FLOWLENS_AUTH_ACCESS_KEY", "environment-access-key")

	cfg := parseExample(t)
	if cfg.Server.Listen != "0.0.0.0:8080" {
		t.Fatalf("environment changed server listen to %q", cfg.Server.Listen)
	}
	if cfg.ClashAPI.Secret.Value() == "environment-clash-secret" {
		t.Fatal("environment overrode clash api secret")
	}
	if cfg.Auth.AccessKey.Value() == "environment-access-key" {
		t.Fatal("environment overrode access key")
	}
}

func TestSecretsRedactByDefault(t *testing.T) {
	cfg := parseExample(t)
	rawSecrets := []string{cfg.ClashAPI.Secret.Value(), cfg.Auth.AccessKey.Value()}

	formatted := fmt.Sprintf("%+v", cfg)
	marshaled, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal secrets: %v", err)
	}

	for _, raw := range rawSecrets {
		if strings.Contains(formatted, raw) {
			t.Fatal("formatted secret leaked its raw value")
		}
		if strings.Contains(string(marshaled), raw) {
			t.Fatal("marshaled secret leaked its raw value")
		}
	}
}
