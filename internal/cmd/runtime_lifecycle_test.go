package cmd

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/spf13/viper"

	"github.com/futuretea/etcd-mcp-server/pkg/core/config"
	mcpserver "github.com/futuretea/etcd-mcp-server/pkg/server/mcp"
	"github.com/futuretea/etcd-mcp-server/pkg/toolset/etcd"
)

func TestToolsCallCreatesAndClosesOneRuntimeClient(t *testing.T) {
	original := newRuntimeClient
	closer := &runtimeTrackingCloser{}
	adapter := &runtimeRecordingAdapter{}
	newRuntimeClient = func(*config.StaticConfig) (runtimeClient, error) {
		return runtimeClient{adapter: adapter, closer: closer}, nil
	}
	t.Cleanup(func() { newRuntimeClient = original })

	var output bytes.Buffer
	root := NewRootCommand(IOStreams{Out: &output, ErrOut: &output})
	root.SetArgs([]string{"--etcd-endpoints", "https://127.0.0.1:2379", "tools", "call", "etcd_lease_list", "--params", "{}"})
	if err := root.Execute(); err != nil {
		t.Fatalf("tools call: %v", err)
	}
	if adapter.calls != 1 || closer.calls != 1 {
		t.Fatalf("adapter calls=%d closer calls=%d, want one each", adapter.calls, closer.calls)
	}
}

func TestRunServerClosesRuntimeClientAfterStdioReturn(t *testing.T) {
	closer := installRuntimeLifecycleFakes(t)
	serveMCPStdio = func(*mcpserver.Server) error { return nil }

	if err := runServer(t.Context(), "", IOStreams{}, viper.New()); err != nil {
		t.Fatalf("runServer: %v", err)
	}
	if closer.calls != 1 {
		t.Fatalf("client Close calls = %d, want 1", closer.calls)
	}
}

func TestRunServerClosesRuntimeClientWhenServerConstructionFails(t *testing.T) {
	closer := installRuntimeLifecycleFakes(t)
	newMCPServer = func(mcpserver.Configuration) (*mcpserver.Server, error) {
		return nil, errors.New("server construction failed")
	}

	err := runServer(t.Context(), "", IOStreams{}, viper.New())
	if err == nil || !strings.Contains(err.Error(), "server construction failed") {
		t.Fatalf("runServer error = %v", err)
	}
	if closer.calls != 1 {
		t.Fatalf("client Close calls = %d, want 1", closer.calls)
	}
}

func installRuntimeLifecycleFakes(t *testing.T) *runtimeTrackingCloser {
	t.Helper()
	originalRuntime := newRuntimeClient
	originalLoad := loadServerConfig
	originalServer := newMCPServer
	originalStdio := serveMCPStdio
	t.Cleanup(func() {
		newRuntimeClient = originalRuntime
		loadServerConfig = originalLoad
		newMCPServer = originalServer
		serveMCPStdio = originalStdio
	})

	closer := &runtimeTrackingCloser{}
	newRuntimeClient = func(*config.StaticConfig) (runtimeClient, error) {
		return runtimeClient{adapter: &runtimeRecordingAdapter{}, closer: closer}, nil
	}
	loadServerConfig = func(string, *viper.Viper) (*config.StaticConfig, error) {
		return &config.StaticConfig{LogLevel: "disabled", EtcdEndpoints: []string{"https://127.0.0.1:2379"}}, nil
	}
	return closer
}

type runtimeRecordingAdapter struct{ calls int }

func (a *runtimeRecordingAdapter) Do(context.Context, etcd.Request) (any, error) {
	a.calls++
	return map[string]any{"leases": []any{}}, nil
}

type runtimeTrackingCloser struct{ calls int }

func (c *runtimeTrackingCloser) Close() error {
	c.calls++
	return nil
}
