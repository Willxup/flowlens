package config_test

import (
	"strings"
	"testing"
	"time"

	"github.com/Willxup/flowlens/internal/config"
)

func TestValidateRejectsInvalidConfigurations(t *testing.T) {
	tests := []struct {
		name   string
		field  string
		mutate func(*config.Config)
	}{
		{name: "schema version", field: "schema_version", mutate: func(c *config.Config) { c.SchemaVersion = 2 }},
		{name: "listen missing host", field: "server.listen", mutate: func(c *config.Config) { c.Server.Listen = ":8080" }},
		{name: "listen missing port", field: "server.listen", mutate: func(c *config.Config) { c.Server.Listen = "0.0.0.0" }},
		{name: "listen host contains whitespace", field: "server.listen", mutate: func(c *config.Config) { c.Server.Listen = "bad host:8080" }},
		{name: "listen non numeric port", field: "server.listen", mutate: func(c *config.Config) { c.Server.Listen = "0.0.0.0:http" }},
		{name: "listen zero port", field: "server.listen", mutate: func(c *config.Config) { c.Server.Listen = "0.0.0.0:0" }},
		{name: "listen port too large", field: "server.listen", mutate: func(c *config.Config) { c.Server.Listen = "0.0.0.0:65536" }},
		{name: "clash api https", field: "clash_api.url", mutate: func(c *config.Config) { c.ClashAPI.URL = "https://sing-box:9090" }},
		{name: "clash api missing port", field: "clash_api.url", mutate: func(c *config.Config) { c.ClashAPI.URL = "http://sing-box" }},
		{name: "clash api path", field: "clash_api.url", mutate: func(c *config.Config) { c.ClashAPI.URL = "http://sing-box:9090/connections" }},
		{name: "clash api userinfo", field: "clash_api.url", mutate: func(c *config.Config) { c.ClashAPI.URL = "http://user:pass@sing-box:9090" }},
		{name: "clash api query", field: "clash_api.url", mutate: func(c *config.Config) { c.ClashAPI.URL = "http://sing-box:9090?debug=1" }},
		{name: "clash api fragment", field: "clash_api.url", mutate: func(c *config.Config) { c.ClashAPI.URL = "http://sing-box:9090#fragment" }},
		{name: "clash api empty secret", field: "clash_api.secret", mutate: func(c *config.Config) { c.ClashAPI.Secret = "" }},
		{name: "clash api whitespace secret", field: "clash_api.secret", mutate: func(c *config.Config) { c.ClashAPI.Secret = "   " }},
		{name: "request timeout zero", field: "clash_api.request_timeout", mutate: func(c *config.Config) { c.ClashAPI.RequestTimeout = config.Duration{} }},
		{name: "request timeout negative", field: "clash_api.request_timeout", mutate: func(c *config.Config) { c.ClashAPI.RequestTimeout = config.Duration{Duration: -time.Second} }},
		{name: "connections interval zero", field: "clash_api.connections_interval", mutate: func(c *config.Config) { c.ClashAPI.ConnectionsInterval = config.Duration{} }},
		{name: "max response size zero", field: "clash_api.max_response_size", mutate: func(c *config.Config) { c.ClashAPI.MaxResponseSize = 0 }},
		{name: "access key short", field: "auth.access_key", mutate: func(c *config.Config) { c.Auth.AccessKey = config.Secret(strings.Repeat("密", 15)) }},
		{name: "access key whitespace", field: "auth.access_key", mutate: func(c *config.Config) { c.Auth.AccessKey = config.Secret(strings.Repeat(" ", 16)) }},
		{name: "session ttl zero", field: "auth.session_ttl", mutate: func(c *config.Config) { c.Auth.SessionTTL = config.Duration{} }},
		{name: "database path relative", field: "storage.database_path", mutate: func(c *config.Config) { c.Storage.DatabasePath = "data/flowlens.db" }},
		{name: "database path outside", field: "storage.database_path", mutate: func(c *config.Config) { c.Storage.DatabasePath = "/var/lib/flowlens-evil/flowlens.db" }},
		{name: "database path is root", field: "storage.database_path", mutate: func(c *config.Config) { c.Storage.DatabasePath = "/var/lib/flowlens" }},
		{name: "soft limit zero", field: "storage.soft_limit", mutate: func(c *config.Config) { c.Storage.SoftLimit = 0 }},
		{name: "timezone empty", field: "time.timezone", mutate: func(c *config.Config) { c.Time.Timezone = "" }},
		{name: "timezone unknown", field: "time.timezone", mutate: func(c *config.Config) { c.Time.Timezone = "Mars/Olympus" }},
		{name: "ten second retention zero", field: "retention.ten_second_days", mutate: func(c *config.Config) { c.Retention.TenSecondDays = 0 }},
		{name: "minute retention negative", field: "retention.minute_days", mutate: func(c *config.Config) { c.Retention.MinuteDays = -1 }},
		{name: "half hour retention zero", field: "retention.half_hour_days", mutate: func(c *config.Config) { c.Retention.HalfHourDays = 0 }},
		{name: "hour retention zero", field: "retention.hour_days", mutate: func(c *config.Config) { c.Retention.HourDays = 0 }},
		{name: "top k zero", field: "retention.top_k", mutate: func(c *config.Config) { c.Retention.TopK = 0 }},
		{name: "top k too large", field: "retention.top_k", mutate: func(c *config.Config) { c.Retention.TopK = 101 }},
		{name: "source mode unknown", field: "privacy.source_mode", mutate: func(c *config.Config) { c.Privacy.SourceMode = "hashed" }},
		{name: "ipv4 prefix negative", field: "privacy.source_ipv4_prefix", mutate: func(c *config.Config) { c.Privacy.SourceIPv4Prefix = -1 }},
		{name: "ipv4 prefix too large", field: "privacy.source_ipv4_prefix", mutate: func(c *config.Config) { c.Privacy.SourceIPv4Prefix = 33 }},
		{name: "ipv6 prefix negative", field: "privacy.source_ipv6_prefix", mutate: func(c *config.Config) { c.Privacy.SourceIPv6Prefix = -1 }},
		{name: "ipv6 prefix too large", field: "privacy.source_ipv6_prefix", mutate: func(c *config.Config) { c.Privacy.SourceIPv6Prefix = 129 }},
		{name: "backup directory relative", field: "backup.directory", mutate: func(c *config.Config) { c.Backup.Directory = "backups" }},
		{name: "backup directory outside", field: "backup.directory", mutate: func(c *config.Config) { c.Backup.Directory = "/var/lib/flowlens-evil/backups" }},
		{name: "backup time invalid", field: "backup.local_time", mutate: func(c *config.Config) { c.Backup.LocalTime = config.ClockTime{Hour: 24} }},
		{name: "daily keep zero", field: "backup.daily_keep", mutate: func(c *config.Config) { c.Backup.DailyKeep = 0 }},
		{name: "monthly keep negative", field: "backup.monthly_keep", mutate: func(c *config.Config) { c.Backup.MonthlyKeep = -1 }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg := parseExample(t)
			cfg.ClashAPI.Secret = config.Secret("clash-secret-must-not-leak")
			cfg.Auth.AccessKey = config.Secret("access-key-must-not-leak")
			test.mutate(&cfg)

			err := config.Validate(&cfg)
			if err == nil {
				t.Fatalf("Validate accepted invalid %s", test.field)
			}
			if !strings.Contains(err.Error(), test.field) {
				t.Fatalf("error %q does not name field %s", err, test.field)
			}
			for _, secret := range []string{"clash-secret-must-not-leak", "access-key-must-not-leak"} {
				if strings.Contains(err.Error(), secret) {
					t.Fatalf("error leaked secret %q", secret)
				}
			}
		})
	}
}

func TestValidateAcceptsBoundariesAndNormalizesURL(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*config.Config)
	}{
		{name: "port one", mutate: func(c *config.Config) { c.Server.Listen = "127.0.0.1:1" }},
		{name: "port max", mutate: func(c *config.Config) { c.Server.Listen = "[::1]:65535" }},
		{name: "top k one", mutate: func(c *config.Config) { c.Retention.TopK = 1 }},
		{name: "top k one hundred", mutate: func(c *config.Config) { c.Retention.TopK = 100 }},
		{name: "ipv4 zero", mutate: func(c *config.Config) { c.Privacy.SourceIPv4Prefix = 0 }},
		{name: "ipv4 max", mutate: func(c *config.Config) { c.Privacy.SourceIPv4Prefix = 32 }},
		{name: "ipv6 zero", mutate: func(c *config.Config) { c.Privacy.SourceIPv6Prefix = 0 }},
		{name: "ipv6 max", mutate: func(c *config.Config) { c.Privacy.SourceIPv6Prefix = 128 }},
		{name: "source full", mutate: func(c *config.Config) { c.Privacy.SourceMode = "full" }},
		{name: "source disabled", mutate: func(c *config.Config) { c.Privacy.SourceMode = "disabled" }},
		{name: "timezone UTC", mutate: func(c *config.Config) { c.Time.Timezone = "UTC" }},
		{name: "timezone non whole hour", mutate: func(c *config.Config) { c.Time.Timezone = "Australia/Eucla" }},
		{name: "timezone DST", mutate: func(c *config.Config) { c.Time.Timezone = "America/New_York" }},
		{name: "access key sixteen unicode characters", mutate: func(c *config.Config) { c.Auth.AccessKey = config.Secret(strings.Repeat("密", 16)) }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg := parseExample(t)
			test.mutate(&cfg)
			if err := config.Validate(&cfg); err != nil {
				t.Fatalf("Validate rejected boundary: %v", err)
			}
		})
	}

	cfg := parseExample(t)
	cfg.ClashAPI.URL = "http://sing-box:9090/"
	if err := config.Validate(&cfg); err != nil {
		t.Fatalf("Validate rejected root trailing slash: %v", err)
	}
	if cfg.ClashAPI.URL != "http://sing-box:9090" {
		t.Fatalf("normalized URL = %q", cfg.ClashAPI.URL)
	}
}

func TestParseRejectsInvalidScalars(t *testing.T) {
	tests := []struct {
		name string
		old  string
		new  string
	}{
		{name: "duration syntax", old: `request_timeout: "3s"`, new: `request_timeout: "soon"`},
		{name: "duration zero", old: `request_timeout: "3s"`, new: `request_timeout: "0s"`},
		{name: "duration negative", old: `request_timeout: "3s"`, new: `request_timeout: "-1s"`},
		{name: "size decimal unit", old: `max_response_size: "16MiB"`, new: `max_response_size: "16MB"`},
		{name: "size zero", old: `max_response_size: "16MiB"`, new: `max_response_size: "0MiB"`},
		{name: "size leading zero", old: `max_response_size: "16MiB"`, new: `max_response_size: "016MiB"`},
		{name: "size explicit plus", old: `max_response_size: "16MiB"`, new: `max_response_size: "+16MiB"`},
		{name: "size overflow", old: `max_response_size: "16MiB"`, new: `max_response_size: "9223372036854775807GiB"`},
		{name: "time non canonical", old: `local_time: "04:00"`, new: `local_time: "4:00"`},
		{name: "time explicit plus", old: `local_time: "04:00"`, new: `local_time: "+4:00"`},
		{name: "time out of range", old: `local_time: "04:00"`, new: `local_time: "24:00"`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := strings.Replace(exampleYAML(t), test.old, test.new, 1)
			if _, err := config.Parse(strings.NewReader(input)); err == nil {
				t.Fatal("Parse accepted an invalid scalar")
			}
		})
	}
}
