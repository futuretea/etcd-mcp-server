package config

import (
	"strings"
	"testing"
	"time"

	"github.com/spf13/viper"
)

// Seam: LoadConfig and StaticConfig.Validate are the startup-only boundary for
// etcd connection inputs. An empty endpoint list remains valid for schema-only
// commands; the factory owns the mandatory-endpoint check.
func TestLoadConfigAppliesEtcdDefaults(t *testing.T) {
	cfg, err := LoadConfig("", viper.New())
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	if cfg.EtcdDialTimeout != 5*time.Second {
		t.Fatalf("EtcdDialTimeout = %s, want 5s", cfg.EtcdDialTimeout)
	}
	if cfg.EtcdRequestTimeout != 10*time.Second {
		t.Fatalf("EtcdRequestTimeout = %s, want 10s", cfg.EtcdRequestTimeout)
	}
	if cfg.EtcdMaxItems != 100 {
		t.Fatalf("EtcdMaxItems = %d, want 100", cfg.EtcdMaxItems)
	}
	if cfg.EtcdMaxResponseBytes != 1<<20 {
		t.Fatalf("EtcdMaxResponseBytes = %d, want 1048576", cfg.EtcdMaxResponseBytes)
	}
}

// Seam: validation must fail before a client can dial when durations, output
// limits, or the certificate/key pair are incomplete.
func TestStaticConfigValidateRejectsInvalidEtcdStartupInputs(t *testing.T) {
	valid := StaticConfig{
		LogLevel:             "info",
		Listen:               "127.0.0.1",
		EtcdDialTimeout:      5 * time.Second,
		EtcdRequestTimeout:   10 * time.Second,
		EtcdMaxItems:         100,
		EtcdMaxResponseBytes: 1 << 20,
	}

	tests := []struct {
		name string
		edit func(*StaticConfig)
	}{
		{
			name: "zero dial timeout",
			edit: func(cfg *StaticConfig) { cfg.EtcdDialTimeout = 0 },
		},
		{
			name: "negative request timeout",
			edit: func(cfg *StaticConfig) { cfg.EtcdRequestTimeout = -time.Second },
		},
		{
			name: "zero maximum items",
			edit: func(cfg *StaticConfig) { cfg.EtcdMaxItems = 0 },
		},
		{
			name: "negative maximum response bytes",
			edit: func(cfg *StaticConfig) { cfg.EtcdMaxResponseBytes = -1 },
		},
		{
			name: "certificate without key",
			edit: func(cfg *StaticConfig) { cfg.EtcdCertFile = "client.pem" },
		},
		{
			name: "key without certificate",
			edit: func(cfg *StaticConfig) { cfg.EtcdKeyFile = "client-key.pem" },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := valid
			tt.edit(&cfg)
			if err := cfg.Validate(); err == nil {
				t.Fatal("Validate() error = nil, want fail-fast validation error")
			}
		})
	}
}

func TestStaticConfigValidateAllowsSchemaOnlyConfigWithoutEndpoints(t *testing.T) {
	cfg := StaticConfig{
		LogLevel:             "info",
		Listen:               "127.0.0.1",
		EtcdDialTimeout:      5 * time.Second,
		EtcdRequestTimeout:   10 * time.Second,
		EtcdMaxItems:         100,
		EtcdMaxResponseBytes: 1 << 20,
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("schema-only Validate() error = %v, want nil", err)
	}
}

func TestLoadConfigReadsEtcdConnectionFields(t *testing.T) {
	v := viper.New()
	v.Set("etcd_endpoints", []string{"https://127.0.0.1:2379"})
	v.Set("etcd_dial_timeout", "7s")
	v.Set("etcd_request_timeout", "11s")
	v.Set("etcd_ca_file", "ca.pem")
	v.Set("etcd_max_items", 37)
	v.Set("etcd_max_response_bytes", 2048)

	cfg, err := LoadConfig("", v)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if got := strings.Join(cfg.EtcdEndpoints, ","); got != "https://127.0.0.1:2379" {
		t.Fatalf("EtcdEndpoints = %q", got)
	}
	if cfg.EtcdDialTimeout != 7*time.Second || cfg.EtcdRequestTimeout != 11*time.Second {
		t.Fatalf("timeouts = (%s, %s), want (7s, 11s)", cfg.EtcdDialTimeout, cfg.EtcdRequestTimeout)
	}
	if cfg.EtcdCAFile != "ca.pem" || cfg.EtcdMaxItems != 37 || cfg.EtcdMaxResponseBytes != 2048 {
		t.Fatalf("unexpected etcd config: %+v", cfg)
	}
}
