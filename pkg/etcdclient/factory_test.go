package etcdclient

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/futuretea/etcd-mcp-server/pkg/core/config"
)

// Seam: New is the only owner of live etcd client construction. A nil startup
// configuration must fail before any network operation is possible.
func TestNewRejectsNilStaticConfig(t *testing.T) {
	_, err := New(nil)
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "config") {
		t.Fatalf("New(nil) error = %v, want configuration validation error", err)
	}
}

func TestClientConfigRejectsMissingEndpointAndPreservesReceiveAndRetryLimits(t *testing.T) {
	cfg := validConfig()
	cfg.EtcdEndpoints = nil
	if _, err := clientConfig(cfg); err == nil || !strings.Contains(err.Error(), "etcd_endpoints") {
		t.Fatalf("clientConfig missing endpoints error = %v", err)
	}

	cfg = validConfig()
	clientCfg, err := clientConfig(cfg)
	if err != nil {
		t.Fatalf("clientConfig: %v", err)
	}
	if clientCfg.MaxCallRecvMsgSize != cfg.EtcdMaxResponseBytes {
		t.Fatalf("MaxCallRecvMsgSize = %d, want %d", clientCfg.MaxCallRecvMsgSize, cfg.EtcdMaxResponseBytes)
	}
	if clientCfg.MaxUnaryRetries != 1 {
		t.Fatalf("MaxUnaryRetries = %d, want 1", clientCfg.MaxUnaryRetries)
	}
}

func TestCredentialsFromEnvironmentRequiresPairWithoutEchoingValues(t *testing.T) {
	t.Setenv(usernameEnvironment, "only-user")
	_ = os.Unsetenv(passwordEnvironment)
	_, _, err := credentialsFromEnvironment()
	if err == nil || strings.Contains(err.Error(), "only-user") {
		t.Fatalf("single credential error = %v, want paired validation without value", err)
	}

	t.Setenv(passwordEnvironment, "secret-value")
	username, password, err := credentialsFromEnvironment()
	if err != nil || username != "only-user" || password != "secret-value" {
		t.Fatalf("credentials = (%q, %q, %v)", username, password, err)
	}
}

func TestClientConfigPropagatesPairedEnvironmentCredentials(t *testing.T) {
	t.Setenv(usernameEnvironment, "reader")
	t.Setenv(passwordEnvironment, "test-password")

	clientCfg, err := clientConfig(validConfig())
	if err != nil {
		t.Fatalf("clientConfig: %v", err)
	}
	if clientCfg.Username != "reader" || clientCfg.Password != "test-password" {
		t.Fatalf("client credentials were not propagated")
	}
}

func TestLoadTLSConfigRejectsMissingCAFile(t *testing.T) {
	cfg := validConfig()
	cfg.EtcdCAFile = "missing-ca.pem"
	if _, err := loadTLSConfig(cfg); err == nil || !strings.Contains(err.Error(), "etcd_ca_file") {
		t.Fatalf("loadTLSConfig error = %v, want CA file error", err)
	}
}

func validConfig() *config.StaticConfig {
	return &config.StaticConfig{
		LogLevel: "info", Listen: "127.0.0.1", EtcdEndpoints: []string{"https://127.0.0.1:2379"},
		EtcdDialTimeout: 5 * time.Second, EtcdRequestTimeout: 10 * time.Second,
		EtcdMaxItems: 100, EtcdMaxResponseBytes: 2048,
	}
}
