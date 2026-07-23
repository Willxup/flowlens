package config

const Path = "/etc/flowlens/config.yaml"

type Config struct {
	SchemaVersion int       `yaml:"schema_version"`
	Server        Server    `yaml:"server"`
	ClashAPI      ClashAPI  `yaml:"clash_api"`
	Auth          Auth      `yaml:"auth"`
	Storage       Storage   `yaml:"storage"`
	Time          Time      `yaml:"time"`
	Retention     Retention `yaml:"retention"`
	Privacy       Privacy   `yaml:"privacy"`
	Backup        Backup    `yaml:"backup"`
}

type Server struct {
	Listen string `yaml:"listen"`
}

type ClashAPI struct {
	URL                 string   `yaml:"url"`
	Secret              Secret   `yaml:"secret"`
	RequestTimeout      Duration `yaml:"request_timeout"`
	ConnectionsInterval Duration `yaml:"connections_interval"`
	MaxResponseSize     ByteSize `yaml:"max_response_size"`
}

type Auth struct {
	Enabled    bool     `yaml:"enabled"`
	AccessKey  Secret   `yaml:"access_key"`
	SessionTTL Duration `yaml:"session_ttl"`
}

type Storage struct {
	DatabasePath string   `yaml:"database_path"`
	SoftLimit    ByteSize `yaml:"soft_limit"`
}

type Time struct {
	Timezone string `yaml:"timezone"`
}

type Retention struct {
	TenSecondDays int `yaml:"ten_second_days"`
	MinuteDays    int `yaml:"minute_days"`
	HalfHourDays  int `yaml:"half_hour_days"`
	HourDays      int `yaml:"hour_days"`
	TopK          int `yaml:"top_k"`
}

type Privacy struct {
	SourceMode       string `yaml:"source_mode"`
	SourceIPv4Prefix int    `yaml:"source_ipv4_prefix"`
	SourceIPv6Prefix int    `yaml:"source_ipv6_prefix"`
}

type Backup struct {
	Directory   string    `yaml:"directory"`
	LocalTime   ClockTime `yaml:"local_time"`
	DailyKeep   int       `yaml:"daily_keep"`
	MonthlyKeep int       `yaml:"monthly_keep"`
}
