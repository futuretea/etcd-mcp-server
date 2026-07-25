package mcp

import (
	"context"
	"testing"

	mcpprotocol "github.com/mark3labs/mcp-go/mcp"

	"github.com/futuretea/etcd-mcp-server/pkg/core/config"
	"github.com/futuretea/etcd-mcp-server/pkg/toolset"
)

// Seam: Server owns the composition-layer client closer and releases it once,
// even when Close is called repeatedly by an HTTP or stdio shutdown path.
func TestServerCloseClosesInjectedClientOnce(t *testing.T) {
	closer := &trackingCloser{}
	server, err := NewServer(Configuration{
		StaticConfig: &config.StaticConfig{LogLevel: "info"},
		Toolsets:     []toolset.Toolset{lifecycleToolset{}},
		Closer:       closer,
	})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	server.Close()
	server.Close()
	if closer.calls != 1 {
		t.Fatalf("client Close calls = %d, want 1", closer.calls)
	}
}

type trackingCloser struct {
	calls int
}

func (c *trackingCloser) Close() error {
	c.calls++
	return nil
}

type lifecycleToolset struct{}

func (lifecycleToolset) GetName() string        { return "test" }
func (lifecycleToolset) GetDescription() string { return "test" }
func (lifecycleToolset) GetTools() []toolset.ServerTool {
	return []toolset.ServerTool{{
		Tool: mcpprotocol.NewTool("read_only_test"),
		Handler: func(context.Context, map[string]any) (string, error) {
			return "{}", nil
		},
		Domain: "test",
	}}
}
