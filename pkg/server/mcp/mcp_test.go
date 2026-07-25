package mcp_test

import (
	"strings"
	"testing"

	"github.com/futuretea/etcd-mcp-server/pkg/core/config"
	mcpserver "github.com/futuretea/etcd-mcp-server/pkg/server/mcp"
	"github.com/futuretea/etcd-mcp-server/pkg/toolset"
	"github.com/futuretea/etcd-mcp-server/pkg/toolset/etcd"
)

func TestNewServerRegistersEtcdTools(t *testing.T) {
	server, err := mcpserver.NewServer(mcpserver.Configuration{
		StaticConfig: &config.StaticConfig{LogLevel: "info"},
		Toolsets:     []toolset.Toolset{etcd.NewToolset(nil, etcd.Config{})},
	})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	defer server.Close()

	if !server.IsHealthy() {
		t.Fatal("expected healthy server")
	}
	tools := server.GetEnabledTools()
	if len(tools) != 12 {
		t.Fatalf("expected 12 tools, got %v", tools)
	}
	names := map[string]bool{}
	for _, name := range tools {
		names[name] = true
	}
	if !names["etcd_get"] || !names["etcd_role_list"] {
		t.Fatalf("expected etcd tools, got %v", tools)
	}
}

func TestNewServerRequiresStaticConfig(t *testing.T) {
	_, err := mcpserver.NewServer(mcpserver.Configuration{})
	if err == nil || !strings.Contains(err.Error(), "static config is required") {
		t.Fatalf("expected static config error, got %v", err)
	}
}

func TestNewServerRejectsEmptyToolFilter(t *testing.T) {
	_, err := mcpserver.NewServer(mcpserver.Configuration{
		StaticConfig: &config.StaticConfig{
			LogLevel:     "info",
			EnabledTools: []string{"missing"},
		},
		Toolsets: []toolset.Toolset{etcd.NewToolset(nil, etcd.Config{})},
	})
	if err == nil || !strings.Contains(err.Error(), "no tools registered") {
		t.Fatalf("expected no tools registered error, got %v", err)
	}
}

func TestNewServerRejectsDuplicateToolNames(t *testing.T) {
	_, err := mcpserver.NewServer(mcpserver.Configuration{
		StaticConfig: &config.StaticConfig{LogLevel: "info"},
		Toolsets: []toolset.Toolset{
			etcd.NewToolset(nil, etcd.Config{}),
			etcd.NewToolset(nil, etcd.Config{}),
		},
	})
	if err == nil || !strings.Contains(err.Error(), `duplicate tool name "etcd_get"`) {
		t.Fatalf("expected duplicate tool name error, got %v", err)
	}
}

func TestNewTextResult(t *testing.T) {
	ok := mcpserver.NewTextResult("hello", nil)
	if ok.IsError || len(ok.Content) != 1 {
		t.Fatalf("unexpected ok result: %+v", ok)
	}

	fail := mcpserver.NewTextResult("", errString("boom"))
	if !fail.IsError {
		t.Fatal("expected error result")
	}
}

type errString string

func (e errString) Error() string { return string(e) }
