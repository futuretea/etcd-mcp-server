package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/futuretea/etcd-mcp-server/pkg/core/config"
	internalhttp "github.com/futuretea/etcd-mcp-server/pkg/server/http"
	mcpserver "github.com/futuretea/etcd-mcp-server/pkg/server/mcp"
	"github.com/futuretea/etcd-mcp-server/pkg/toolset"
	"github.com/futuretea/etcd-mcp-server/pkg/toolset/etcd"
)

func TestHealthz(t *testing.T) {
	mcpServer, err := mcpserver.NewServer(mcpserver.Configuration{
		StaticConfig: &config.StaticConfig{LogLevel: "info"},
		Toolsets:     []toolset.Toolset{etcd.NewToolset(nil, etcd.Config{})},
	})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	defer mcpServer.Close()

	httpServer := &http.Server{}
	handler := internalhttp.NewHandler(mcpServer, httpServer, "")

	req := mustNewRequest(t, http.MethodGet, internalhttp.HealthEndpoint)
	rec := &responseRecorder{header: make(http.Header)}
	handler.ServeHTTP(rec, req)
	if rec.status != http.StatusOK || string(rec.body) != "healthy" {
		t.Fatalf("healthy status=%d body=%q", rec.status, rec.body)
	}

	testServer := httptest.NewServer(handler)
	defer testServer.Close()
	client := testServer.Client()
	client.Timeout = 2 * time.Second
	req, err = http.NewRequestWithContext(t.Context(), http.MethodHead, testServer.URL+internalhttp.HealthEndpoint, nil)
	if err != nil {
		t.Fatalf("NewRequest HEAD: %v", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("HEAD healthz: %v", err)
	}
	body, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		t.Fatalf("read HEAD healthz body: %v", err)
	}
	if resp.StatusCode != http.StatusOK || len(body) != 0 || resp.ContentLength != int64(len("healthy")) {
		t.Fatalf("HEAD healthy status=%d content_length=%d body=%q", resp.StatusCode, resp.ContentLength, body)
	}

	req = mustNewRequest(t, http.MethodPost, internalhttp.HealthEndpoint)
	rec = &responseRecorder{header: make(http.Header)}
	handler.ServeHTTP(rec, req)
	if rec.status != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.status)
	}
}

func TestMCPTransportRoutesAreMounted(t *testing.T) {
	mcpServer, err := mcpserver.NewServer(mcpserver.Configuration{
		StaticConfig: &config.StaticConfig{LogLevel: "info"},
		Toolsets:     []toolset.Toolset{etcd.NewToolset(nil, etcd.Config{})},
	})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	defer mcpServer.Close()

	handler := internalhttp.NewHandler(mcpServer, &http.Server{}, "")
	for _, tt := range []struct {
		name   string
		method string
		path   string
		want   int
	}{
		{name: "streamable HTTP", method: http.MethodPost, path: internalhttp.MCPEndpoint},
		{name: "SSE", method: http.MethodPost, path: internalhttp.SSEEndpoint, want: http.StatusMethodNotAllowed},
		{name: "SSE message", method: http.MethodGet, path: internalhttp.SSEMessageEndpoint, want: http.StatusBadRequest},
	} {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, mustNewRequest(t, tt.method, tt.path))
			if rec.Code == http.StatusNotFound {
				t.Fatalf("%s route returned 404", tt.path)
			}
			if tt.want != 0 && rec.Code != tt.want {
				t.Fatalf("%s status = %d, want %d", tt.path, rec.Code, tt.want)
			}
		})
	}
}

func TestStreamableHTTPRouteInitializesAndListsTools(t *testing.T) {
	mcpServer, err := mcpserver.NewServer(mcpserver.Configuration{
		StaticConfig: &config.StaticConfig{LogLevel: "info"},
		Toolsets:     []toolset.Toolset{etcd.NewToolset(nil, etcd.Config{})},
	})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	defer mcpServer.Close()

	testServer := httptest.NewServer(internalhttp.NewHandler(mcpServer, &http.Server{}, ""))
	defer testServer.Close()
	client := testServer.Client()

	initialize := postMCPJSON(t, client, testServer.URL+internalhttp.MCPEndpoint, map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "initialize",
		"params": map[string]any{
			"protocolVersion": mcp.LATEST_PROTOCOL_VERSION,
			"clientInfo":      map[string]any{"name": "test", "version": "1.0"},
		},
	})
	defer func() { _ = initialize.Body.Close() }()
	if initialize.StatusCode != http.StatusOK {
		t.Fatalf("initialize status = %d", initialize.StatusCode)
	}
	var initialized struct {
		Result struct {
			ProtocolVersion string `json:"protocolVersion"`
		} `json:"result"`
	}
	if err := json.NewDecoder(initialize.Body).Decode(&initialized); err != nil {
		t.Fatalf("decode initialize: %v", err)
	}
	if initialized.Result.ProtocolVersion != mcp.LATEST_PROTOCOL_VERSION {
		t.Fatalf("protocol version = %q, want %q", initialized.Result.ProtocolVersion, mcp.LATEST_PROTOCOL_VERSION)
	}

	list := postMCPJSON(t, client, testServer.URL+internalhttp.MCPEndpoint, map[string]any{
		"jsonrpc": "2.0", "id": 2, "method": "tools/list", "params": map[string]any{},
	})
	defer func() { _ = list.Body.Close() }()
	if list.StatusCode != http.StatusOK {
		t.Fatalf("tools/list status = %d", list.StatusCode)
	}
	var listed struct {
		Result struct {
			Tools []json.RawMessage `json:"tools"`
		} `json:"result"`
	}
	if err := json.NewDecoder(list.Body).Decode(&listed); err != nil {
		t.Fatalf("decode tools/list: %v", err)
	}
	if len(listed.Result.Tools) != 12 {
		t.Fatalf("tools/list count = %d, want 12", len(listed.Result.Tools))
	}
}

func postMCPJSON(t *testing.T, client *http.Client, url string, request map[string]any) *http.Response {
	t.Helper()
	body, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	response, err := client.Do(req)
	if err != nil {
		t.Fatalf("MCP request: %v", err)
	}
	return response
}

func TestServeListenerShutdown(t *testing.T) {
	mcpServer, err := mcpserver.NewServer(mcpserver.Configuration{
		StaticConfig: &config.StaticConfig{LogLevel: "info"},
		Toolsets:     []toolset.Toolset{etcd.NewToolset(nil, etcd.Config{})},
	})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	defer mcpServer.Close()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	cfg := &config.StaticConfig{Port: listener.Addr().(*net.TCPAddr).Port, Listen: "127.0.0.1"}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- internalhttp.ServeListener(ctx, mcpServer, cfg, listener)
	}()

	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get("http://" + listener.Addr().String() + internalhttp.HealthEndpoint)
	if err != nil {
		t.Fatalf("GET healthz: %v", err)
	}
	body, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		t.Fatalf("read healthz body: %v", err)
	}
	if resp.StatusCode != http.StatusOK || string(body) != "healthy" {
		t.Fatalf("healthz status=%d body=%q", resp.StatusCode, body)
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("ServeListener: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("ServeListener did not exit after cancel")
	}
}

type responseRecorder struct {
	header http.Header
	status int
	body   []byte
}

func (r *responseRecorder) Header() http.Header { return r.header }
func (r *responseRecorder) Write(b []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	r.body = append(r.body, b...)
	return len(b), nil
}
func (r *responseRecorder) WriteHeader(statusCode int) { r.status = statusCode }

func mustNewRequest(t *testing.T, method, path string) *http.Request {
	t.Helper()
	req, err := http.NewRequest(method, path, nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	return req
}
