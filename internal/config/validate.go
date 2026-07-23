package config

import (
	"fmt"
	"net"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	_ "time/tzdata"
	"unicode/utf8"
)

const (
	dataDirectory               = "/var/lib/flowlens"
	exampleAccessKeyPlaceholder = "请替换为至少16字符的随机登录密钥"
)

func Validate(cfg *Config) error {
	if cfg == nil {
		return fieldError("config", "must not be nil")
	}
	if cfg.SchemaVersion != 1 {
		return fieldError("schema_version", "must be 1")
	}
	if err := validateListen(cfg.Server.Listen); err != nil {
		return err
	}

	normalizedURL, err := validateClashURL(cfg.ClashAPI.URL)
	if err != nil {
		return err
	}
	cfg.ClashAPI.URL = normalizedURL

	if strings.TrimSpace(cfg.ClashAPI.Secret.Value()) == "" {
		return fieldError("clash_api.secret", "must not be empty")
	}
	if cfg.ClashAPI.RequestTimeout.Duration <= 0 {
		return fieldError("clash_api.request_timeout", "must be positive")
	}
	if cfg.ClashAPI.ConnectionsInterval.Duration <= 0 {
		return fieldError("clash_api.connections_interval", "must be positive")
	}
	if cfg.ClashAPI.MaxResponseSize <= 0 {
		return fieldError("clash_api.max_response_size", "must be positive")
	}

	if cfg.Auth.Enabled {
		accessKey := cfg.Auth.AccessKey.Value()
		if strings.TrimSpace(accessKey) == "" || utf8.RuneCountInString(accessKey) < 16 {
			return fieldError("auth.access_key", "must contain at least 16 characters")
		}
		if accessKey == exampleAccessKeyPlaceholder {
			return fieldError("auth.access_key", "must replace the example value")
		}
		if cfg.Auth.SessionTTL.Duration <= 0 {
			return fieldError("auth.session_ttl", "must be positive")
		}
	}

	if err := validateDataPath("storage.database_path", cfg.Storage.DatabasePath, false); err != nil {
		return err
	}
	if cfg.Storage.SoftLimit <= 0 {
		return fieldError("storage.soft_limit", "must be positive")
	}

	if cfg.Time.Timezone == "" {
		return fieldError("time.timezone", "must not be empty")
	}
	if _, err := time.LoadLocation(cfg.Time.Timezone); err != nil {
		return fieldError("time.timezone", "must be a valid IANA timezone")
	}

	if cfg.Retention.TenSecondDays <= 0 {
		return fieldError("retention.ten_second_days", "must be positive")
	}
	if cfg.Retention.MinuteDays <= 0 {
		return fieldError("retention.minute_days", "must be positive")
	}
	if cfg.Retention.HalfHourDays <= 0 {
		return fieldError("retention.half_hour_days", "must be positive")
	}
	if cfg.Retention.HourDays <= 0 {
		return fieldError("retention.hour_days", "must be positive")
	}
	if cfg.Retention.TopK < 1 || cfg.Retention.TopK > 100 {
		return fieldError("retention.top_k", "must be between 1 and 100")
	}

	switch cfg.Privacy.SourceMode {
	case "full", "prefix", "disabled":
	default:
		return fieldError("privacy.source_mode", "must be full, prefix, or disabled")
	}
	if cfg.Privacy.SourceIPv4Prefix < 0 || cfg.Privacy.SourceIPv4Prefix > 32 {
		return fieldError("privacy.source_ipv4_prefix", "must be between 0 and 32")
	}
	if cfg.Privacy.SourceIPv6Prefix < 0 || cfg.Privacy.SourceIPv6Prefix > 128 {
		return fieldError("privacy.source_ipv6_prefix", "must be between 0 and 128")
	}

	if err := validateDataPath("backup.directory", cfg.Backup.Directory, true); err != nil {
		return err
	}
	if cfg.Backup.LocalTime.Hour < 0 || cfg.Backup.LocalTime.Hour > 23 || cfg.Backup.LocalTime.Minute < 0 || cfg.Backup.LocalTime.Minute > 59 {
		return fieldError("backup.local_time", "must use a time from 00:00 through 23:59")
	}
	if cfg.Backup.DailyKeep <= 0 {
		return fieldError("backup.daily_keep", "must be positive")
	}
	if cfg.Backup.MonthlyKeep <= 0 {
		return fieldError("backup.monthly_keep", "must be positive")
	}

	return nil
}

func validateListen(value string) error {
	host, portText, err := net.SplitHostPort(value)
	if err != nil || strings.TrimSpace(host) == "" || strings.ContainsAny(host, " \t\r\n/") {
		return fieldError("server.listen", "must use host:port with a non-empty host")
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port < 1 || port > 65535 {
		return fieldError("server.listen", "port must be between 1 and 65535")
	}
	return nil
}

func validateClashURL(value string) (string, error) {
	parsed, err := url.ParseRequestURI(value)
	if err != nil || parsed.Scheme != "http" || parsed.Host == "" || parsed.Hostname() == "" {
		return "", fieldError("clash_api.url", "must be an http root URL with an explicit host and port")
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" {
		return "", fieldError("clash_api.url", "must not contain user information, query, or fragment")
	}
	if parsed.Path != "" && parsed.Path != "/" {
		return "", fieldError("clash_api.url", "must not contain a path")
	}
	portText := parsed.Port()
	port, err := strconv.Atoi(portText)
	if err != nil || port < 1 || port > 65535 {
		return "", fieldError("clash_api.url", "must include a port between 1 and 65535")
	}
	return "http://" + parsed.Host, nil
}

func validateDataPath(field string, value string, allowRoot bool) error {
	if !filepath.IsAbs(value) || filepath.Clean(value) != value {
		return fieldError(field, "must be a clean absolute path inside /var/lib/flowlens")
	}
	relative, err := filepath.Rel(dataDirectory, value)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return fieldError(field, "must be inside /var/lib/flowlens")
	}
	if !allowRoot && relative == "." {
		return fieldError(field, "must name a file inside /var/lib/flowlens")
	}
	return nil
}

func fieldError(field string, reason string) error {
	return fmt.Errorf("%s: %s", field, reason)
}
