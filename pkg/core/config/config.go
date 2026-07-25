package config

import (
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"github.com/spf13/viper"
)

// StaticConfig contains configuration that is fixed when the server starts.
type StaticConfig struct {
	Port                 int           `mapstructure:"port"`
	Listen               string        `mapstructure:"listen"`
	SSEBaseURL           string        `mapstructure:"sse_base_url"`
	LogLevel             string        `mapstructure:"log_level"`
	EnabledTools         []string      `mapstructure:"enabled_tools"`
	DisabledTools        []string      `mapstructure:"disabled_tools"`
	EnabledDomains       []string      `mapstructure:"enabled_domains"`
	DisabledDomains      []string      `mapstructure:"disabled_domains"`
	EtcdEndpoints        []string      `mapstructure:"etcd_endpoints"`
	EtcdDialTimeout      time.Duration `mapstructure:"etcd_dial_timeout"`
	EtcdRequestTimeout   time.Duration `mapstructure:"etcd_request_timeout"`
	EtcdCAFile           string        `mapstructure:"etcd_ca_file"`
	EtcdCertFile         string        `mapstructure:"etcd_cert_file"`
	EtcdKeyFile          string        `mapstructure:"etcd_key_file"`
	EtcdMaxItems         int           `mapstructure:"etcd_max_items"`
	EtcdMaxResponseBytes int           `mapstructure:"etcd_max_response_bytes"`
}

// Validate checks whether the configuration can be used to start the server.
func (c *StaticConfig) Validate() error {
	if c.Port < 0 || c.Port > 65535 {
		return fmt.Errorf("port must be between 0 and 65535, got %d", c.Port)
	}
	if c.Port != 0 && strings.TrimSpace(c.Listen) == "" {
		return fmt.Errorf("listen must be set when port is non-zero")
	}
	if _, err := zerolog.ParseLevel(c.LogLevel); err != nil {
		return fmt.Errorf("invalid log_level %q: %w", c.LogLevel, err)
	}
	if c.EtcdDialTimeout <= 0 {
		return fmt.Errorf("etcd_dial_timeout must be positive")
	}
	if c.EtcdRequestTimeout <= 0 {
		return fmt.Errorf("etcd_request_timeout must be positive")
	}
	if c.EtcdMaxItems <= 0 {
		return fmt.Errorf("etcd_max_items must be positive")
	}
	if c.EtcdMaxResponseBytes <= 0 {
		return fmt.Errorf("etcd_max_response_bytes must be positive")
	}
	if (c.EtcdCertFile == "") != (c.EtcdKeyFile == "") {
		return fmt.Errorf("etcd_cert_file and etcd_key_file must be set together")
	}
	return nil
}

// GetListenAddress returns the configured HTTP listen address (host:port).
func (c *StaticConfig) GetListenAddress() string {
	if c.Port == 0 {
		return ""
	}
	return net.JoinHostPort(c.Listen, strconv.Itoa(c.Port))
}

// LoadConfig loads configuration from defaults, optional YAML, environment, and flags.
func LoadConfig(configPath string, v *viper.Viper) (*StaticConfig, error) {
	if v == nil {
		v = viper.New()
	}

	defaults := map[string]any{
		"port":                    0,
		"listen":                  "127.0.0.1",
		"sse_base_url":            "",
		"log_level":               "info",
		"enabled_tools":           []string{},
		"disabled_tools":          []string{},
		"enabled_domains":         []string{},
		"disabled_domains":        []string{},
		"etcd_endpoints":          []string{},
		"etcd_dial_timeout":       5 * time.Second,
		"etcd_request_timeout":    10 * time.Second,
		"etcd_ca_file":            "",
		"etcd_cert_file":          "",
		"etcd_key_file":           "",
		"etcd_max_items":          100,
		"etcd_max_response_bytes": 1 << 20,
	}
	for key, value := range defaults {
		v.SetDefault(key, value)
	}

	if configPath != "" {
		v.SetConfigFile(configPath)
		v.SetConfigType("yaml")
		if err := v.ReadInConfig(); err != nil {
			return nil, fmt.Errorf("read config file: %w", err)
		}
	}

	v.SetEnvPrefix("MCP")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_", "-", "_"))
	v.AutomaticEnv()

	cfg := &StaticConfig{}
	if err := v.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}

	cfg.Listen = strings.TrimSpace(cfg.Listen)
	cfg.LogLevel = strings.ToLower(strings.TrimSpace(cfg.LogLevel))
	cfg.EnabledTools = normalizeStringSlice(cfg.EnabledTools)
	cfg.DisabledTools = normalizeStringSlice(cfg.DisabledTools)
	cfg.EnabledDomains = normalizeStringSlice(cfg.EnabledDomains)
	cfg.DisabledDomains = normalizeStringSlice(cfg.DisabledDomains)
	cfg.EtcdEndpoints = normalizeStringSlice(cfg.EtcdEndpoints)
	cfg.EtcdCAFile = strings.TrimSpace(cfg.EtcdCAFile)
	cfg.EtcdCertFile = strings.TrimSpace(cfg.EtcdCertFile)
	cfg.EtcdKeyFile = strings.TrimSpace(cfg.EtcdKeyFile)

	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

func normalizeStringSlice(values []string) []string {
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			normalized = append(normalized, value)
		}
	}
	return normalized
}
