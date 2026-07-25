//go:build integration

package integration_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/futuretea/etcd-mcp-server/pkg/core/config"
	"github.com/futuretea/etcd-mcp-server/pkg/etcdclient"
	"github.com/futuretea/etcd-mcp-server/pkg/toolset"
	"github.com/futuretea/etcd-mcp-server/pkg/toolset/etcd"
)

func TestEtcdIntegrationReadOnlyTools(t *testing.T) {
	cfg, ok := integrationConfig(t)
	if !ok {
		return
	}
	client, err := etcdclient.New(cfg)
	if err != nil {
		t.Fatalf("create integration client: %v", err)
	}
	defer func() { _ = client.Close() }()

	tools := etcd.NewToolset(etcd.NewV3Adapter(client), etcd.Config{
		Endpoints: cfg.EtcdEndpoints, RequestTimeout: cfg.EtcdRequestTimeout,
		MaxItems: cfg.EtcdMaxItems, MaxResponseBytes: cfg.EtcdMaxResponseBytes,
	})
	call := integrationCaller(t, tools)
	endpoint := cfg.EtcdEndpoints[0]

	for name, params := range map[string]map[string]any{
		"etcd_lease_list":  {},
		"etcd_member_list": {},
		"etcd_alarm_list":  {},
		"etcd_status":      {"endpoint": endpoint},
		"etcd_hash_kv":     {"endpoint": endpoint},
		"etcd_auth_status": {},
	} {
		t.Run(name, func(t *testing.T) {
			var output map[string]any
			call(name, params, &output)
			if _, ok := output["header"]; !ok {
				t.Fatalf("%s output has no header: %s", name, output)
			}
		})
	}

	if encodedKey := os.Getenv("MCP_ETCD_INTEGRATION_GET_KEY_BASE64"); encodedKey == "" {
		t.Run("etcd_get", func(t *testing.T) { t.Skip("MCP_ETCD_INTEGRATION_GET_KEY_BASE64 is not set") })
	} else {
		t.Run("etcd_get", func(t *testing.T) {
			if _, err := base64.StdEncoding.DecodeString(encodedKey); err != nil {
				t.Fatalf("MCP_ETCD_INTEGRATION_GET_KEY_BASE64: %v", err)
			}
			var output struct {
				KVs []struct {
					Key string `json:"key_base64"`
				} `json:"kvs"`
			}
			call("etcd_get", map[string]any{"key_base64": encodedKey, "limit": "1"}, &output)
			if len(output.KVs) == 0 || output.KVs[0].Key != encodedKey {
				t.Fatalf("get result does not return fixture key %q: %+v", encodedKey, output.KVs)
			}
		})
	}

	fixtureSubtest(t, "etcd_lease_ttl", "MCP_ETCD_INTEGRATION_LEASE_ID", func(value string) {
		var output struct {
			LeaseID string `json:"lease_id"`
		}
		call("etcd_lease_ttl", map[string]any{"lease_id": value}, &output)
		if output.LeaseID != value {
			t.Fatalf("lease_id = %q, want %q", output.LeaseID, value)
		}
	})
	fixtureSubtest(t, "etcd_user_get", "MCP_ETCD_INTEGRATION_USER", func(value string) {
		var output struct {
			Roles []string `json:"roles"`
		}
		call("etcd_user_get", map[string]any{"user": value}, &output)
		if output.Roles == nil {
			t.Fatal("user_get roles is null")
		}
	})
	fixtureSubtest(t, "etcd_role_get", "MCP_ETCD_INTEGRATION_ROLE", func(value string) {
		var output struct {
			Permissions []any `json:"permissions"`
		}
		call("etcd_role_get", map[string]any{"role": value}, &output)
		if output.Permissions == nil {
			t.Fatal("role_get permissions is null")
		}
	})

	var auth struct {
		Enabled bool `json:"enabled"`
	}
	call("etcd_auth_status", map[string]any{}, &auth)
	if !auth.Enabled {
		t.Run("etcd_user_list", func(t *testing.T) { t.Skip("etcd authentication is disabled") })
		t.Run("etcd_role_list", func(t *testing.T) { t.Skip("etcd authentication is disabled") })
		return
	}
	integrationListFixture(t, "etcd_user_list", "MCP_ETCD_INTEGRATION_USER", tools, func(output map[string]any, fixture string) bool {
		return stringSliceContains(output["users"], fixture)
	})
	integrationListFixture(t, "etcd_role_list", "MCP_ETCD_INTEGRATION_ROLE", tools, func(output map[string]any, fixture string) bool {
		return stringSliceContains(output["roles"], fixture)
	})
}

func integrationConfig(t *testing.T) (*config.StaticConfig, bool) {
	t.Helper()
	endpoints := strings.FieldsFunc(os.Getenv("MCP_ETCD_INTEGRATION_ENDPOINTS"), func(r rune) bool { return r == ',' || r == ' ' })
	if len(endpoints) == 0 {
		t.Skip("MCP_ETCD_INTEGRATION_ENDPOINTS is not set")
		return nil, false
	}
	cfg := &config.StaticConfig{
		LogLevel: "disabled", Listen: "127.0.0.1", EtcdEndpoints: endpoints,
		EtcdDialTimeout: 5 * time.Second, EtcdRequestTimeout: 10 * time.Second,
		EtcdCAFile:   os.Getenv("MCP_ETCD_INTEGRATION_CA_FILE"),
		EtcdCertFile: os.Getenv("MCP_ETCD_INTEGRATION_CERT_FILE"),
		EtcdKeyFile:  os.Getenv("MCP_ETCD_INTEGRATION_KEY_FILE"),
		EtcdMaxItems: 100, EtcdMaxResponseBytes: 1 << 20,
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("integration configuration: %v", err)
	}
	return cfg, true
}

func integrationCaller(t *testing.T, tools *etcd.Toolset) func(string, map[string]any, any) {
	t.Helper()
	byName := make(map[string]toolset.ServerTool)
	for _, serverTool := range tools.GetTools() {
		byName[serverTool.Tool.Name] = serverTool
	}
	return func(name string, params map[string]any, output any) {
		t.Helper()
		ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
		defer cancel()
		serverTool, ok := byName[name]
		if !ok {
			t.Fatalf("tool %q is not registered", name)
		}
		result, err := serverTool.Handler(ctx, params)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if err := json.Unmarshal([]byte(result), output); err != nil {
			t.Fatalf("decode %s result %q: %v", name, result, err)
		}
	}
}

func fixtureSubtest(t *testing.T, name, environment string, run func(string)) {
	t.Helper()
	value := os.Getenv(environment)
	t.Run(name, func(t *testing.T) {
		if value == "" {
			t.Skip(environment + " is not set")
		}
		run(value)
	})
}

func integrationListFixture(t *testing.T, name, environment string, tools *etcd.Toolset, contains func(map[string]any, string) bool) {
	t.Helper()
	t.Run(name, func(t *testing.T) {
		fixture := os.Getenv(environment)
		if fixture == "" {
			t.Skip(environment + " is not set")
		}
		if os.Getenv("MCP_ETCD_USERNAME") == "" || os.Getenv("MCP_ETCD_PASSWORD") == "" {
			t.Skip("MCP_ETCD_USERNAME and MCP_ETCD_PASSWORD are required for auth list verification")
		}
		result, err := integrationToolCall(t, tools, name, map[string]any{})
		if err != nil {
			if strings.Contains(strings.ToLower(err.Error()), "permission denied") {
				t.Skip("configured credentials cannot list " + name)
			}
			t.Fatalf("%s: %v", name, err)
		}
		var output map[string]any
		if err := json.Unmarshal([]byte(result), &output); err != nil {
			t.Fatalf("decode %s result: %v", name, err)
		}
		if !contains(output, fixture) {
			t.Fatalf("%s result does not contain fixture %q: %s", name, fixture, result)
		}
	})
}

func integrationToolCall(t *testing.T, tools *etcd.Toolset, name string, params map[string]any) (string, error) {
	t.Helper()
	for _, serverTool := range tools.GetTools() {
		if serverTool.Tool.Name != name {
			continue
		}
		ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
		defer cancel()
		return serverTool.Handler(ctx, params)
	}
	return "", &unknownToolError{name: name}
}

func stringSliceContains(value any, want string) bool {
	values, ok := value.([]any)
	if !ok {
		return false
	}
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

type unknownToolError struct {
	name string
}

func (e *unknownToolError) Error() string {
	return "tool is not registered: " + e.name
}
