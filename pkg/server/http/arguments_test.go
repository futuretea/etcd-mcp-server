package http_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/futuretea/etcd-mcp-server/pkg/core/config"
	internalhttp "github.com/futuretea/etcd-mcp-server/pkg/server/http"
	mcpserver "github.com/futuretea/etcd-mcp-server/pkg/server/mcp"
	"github.com/futuretea/etcd-mcp-server/pkg/toolset"
	"github.com/futuretea/etcd-mcp-server/pkg/toolset/etcd"
)

func TestStreamableHTTPRejectsNonObjectArgumentsBeforeEtcdCall(t *testing.T) {
	adapter := &recordingEtcdAdapter{}
	server, err := mcpserver.NewServer(mcpserver.Configuration{
		StaticConfig: &config.StaticConfig{LogLevel: "info"},
		Toolsets: []toolset.Toolset{
			etcd.NewToolset(adapter, etcd.Config{MaxItems: 100, MaxResponseBytes: 1 << 20}),
		},
	})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	defer server.Close()

	httpServer := httptest.NewServer(internalhttp.NewHandler(server, &http.Server{}, ""))
	defer httpServer.Close()
	client := httpServer.Client()

	initializeMCP(t, client, httpServer.URL+internalhttp.MCPEndpoint)

	t.Run("omitted arguments call lease list", func(t *testing.T) {
		before := adapter.calls
		body := callTool(t, client, httpServer.URL+internalhttp.MCPEndpoint, `{"jsonrpc":"2.0","id":"omitted","method":"tools/call","params":{"name":"etcd_lease_list"}}`)
		assertToolSuccess(t, body, "leases")
		if adapter.calls != before+1 || adapter.lastMethod != "lease_list" {
			t.Fatalf("adapter calls=%d method=%q, want one lease_list call", adapter.calls-before, adapter.lastMethod)
		}
	})

	t.Run("null arguments call lease list", func(t *testing.T) {
		before := adapter.calls
		body := callTool(t, client, httpServer.URL+internalhttp.MCPEndpoint, `{"jsonrpc":"2.0","id":"null","method":"tools/call","params":{"name":"etcd_lease_list","arguments":null}}`)
		assertToolSuccess(t, body, "leases")
		if adapter.calls != before+1 {
			t.Fatalf("adapter calls increased by %d, want 1", adapter.calls-before)
		}
	})

	t.Run("empty object calls lease list", func(t *testing.T) {
		before := adapter.calls
		body := callTool(t, client, httpServer.URL+internalhttp.MCPEndpoint, `{"jsonrpc":"2.0","id":"object","method":"tools/call","params":{"name":"etcd_lease_list","arguments":{}}}`)
		assertToolSuccess(t, body, "leases")
		if adapter.calls != before+1 {
			t.Fatalf("adapter calls increased by %d, want 1", adapter.calls-before)
		}
	})

	t.Run("legal get calls etcd", func(t *testing.T) {
		before := adapter.calls
		body := callTool(t, client, httpServer.URL+internalhttp.MCPEndpoint, `{"jsonrpc":"2.0","id":"get","method":"tools/call","params":{"name":"etcd_get","arguments":{"key":"k"}}}`)
		assertToolSuccess(t, body, "kvs")
		if adapter.calls != before+1 || adapter.lastMethod != "get" {
			t.Fatalf("adapter calls=%d method=%q, want one get", adapter.calls-before, adapter.lastMethod)
		}
	})

	for _, tt := range []struct {
		name string
		body string
	}{
		{name: "array", body: `{"jsonrpc":"2.0","id":"array","method":"tools/call","params":{"name":"etcd_lease_list","arguments":[]}}`},
		{name: "array value", body: `{"jsonrpc":"2.0","id":"arrayv","method":"tools/call","params":{"name":"etcd_lease_list","arguments":["x"]}}`},
		{name: "string", body: `{"jsonrpc":"2.0","id":"string","method":"tools/call","params":{"name":"etcd_lease_list","arguments":"x"}}`},
		{name: "number", body: `{"jsonrpc":"2.0","id":"number","method":"tools/call","params":{"name":"etcd_lease_list","arguments":1}}`},
		{name: "bool", body: `{"jsonrpc":"2.0","id":"bool","method":"tools/call","params":{"name":"etcd_lease_list","arguments":true}}`},
		{name: "array get", body: `{"jsonrpc":"2.0","id":"aget","method":"tools/call","params":{"name":"etcd_get","arguments":[]}}`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			before := adapter.calls
			body := callTool(t, client, httpServer.URL+internalhttp.MCPEndpoint, tt.body)
			assertToolError(t, body, "JSON object")
			if adapter.calls != before {
				t.Fatalf("adapter calls = %d, want %d", adapter.calls, before)
			}
		})
	}

	t.Run("unknown tool", func(t *testing.T) {
		before := adapter.calls
		body := callTool(t, client, httpServer.URL+internalhttp.MCPEndpoint, `{"jsonrpc":"2.0","id":"unknown","method":"tools/call","params":{"name":"nope","arguments":{}}}`)
		if !strings.Contains(body, "not found") {
			t.Fatalf("unknown tool body = %s", body)
		}
		if adapter.calls != before {
			t.Fatalf("adapter calls = %d, want %d", adapter.calls, before)
		}
	})

	t.Run("null user field", func(t *testing.T) {
		before := adapter.calls
		body := callTool(t, client, httpServer.URL+internalhttp.MCPEndpoint, `{"jsonrpc":"2.0","id":"nuser","method":"tools/call","params":{"name":"etcd_user_get","arguments":{"user":null}}}`)
		assertToolError(t, body, "string")
		if adapter.calls != before {
			t.Fatalf("adapter calls = %d, want %d", adapter.calls, before)
		}
	})
}

type recordingEtcdAdapter struct {
	calls      int
	lastMethod string
}

func (a *recordingEtcdAdapter) Do(_ context.Context, request etcd.Request) (any, error) {
	a.calls++
	a.lastMethod = request.Method
	if request.Method == "get" {
		return etcd.GetResponse{KVs: []etcd.KeyValue{}}, nil
	}
	return etcd.LeaseListResponse{Leases: []etcd.Lease{}}, nil
}

func initializeMCP(t *testing.T, client *http.Client, url string) {
	t.Helper()
	body := callTool(t, client, url, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"`+mcp.LATEST_PROTOCOL_VERSION+`","capabilities":{},"clientInfo":{"name":"test","version":"0"}}}`)
	if !strings.Contains(body, mcp.LATEST_PROTOCOL_VERSION) {
		t.Fatalf("initialize body = %s", body)
	}
}

func callTool(t *testing.T, client *http.Client, url, payload string) string {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, url, strings.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := io.ReadAll(resp.Body)
	closeErr := resp.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if closeErr != nil {
		t.Fatal(closeErr)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d body = %s", resp.StatusCode, raw)
	}
	return string(raw)
}

func assertToolSuccess(t *testing.T, body, text string) {
	t.Helper()
	var response struct {
		Result struct {
			IsError bool `json:"isError"`
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"result"`
		Error any `json:"error"`
	}
	if err := json.Unmarshal([]byte(body), &response); err != nil {
		t.Fatalf("decode response: %v\n%s", err, body)
	}
	if response.Error != nil || response.Result.IsError || len(response.Result.Content) != 1 || !strings.Contains(response.Result.Content[0].Text, text) {
		t.Fatalf("success body = %s", body)
	}
}

func assertToolError(t *testing.T, body, text string) {
	t.Helper()
	var response struct {
		Result struct {
			IsError bool `json:"isError"`
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(body), &response); err != nil {
		t.Fatalf("decode response: %v\n%s", err, body)
	}
	if !response.Result.IsError || len(response.Result.Content) != 1 || !strings.Contains(response.Result.Content[0].Text, text) {
		t.Fatalf("error body = %s, want text %q", body, text)
	}
}
