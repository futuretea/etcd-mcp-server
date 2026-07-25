package etcd

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
)

func TestGetAcceptsBase64BytesAndCanonicalDecimalStrings(t *testing.T) {
	adapter := &recordingAdapter{response: GetResponse{KVs: []KeyValue{}}}
	tools := NewToolset(adapter, Config{MaxItems: 100, MaxResponseBytes: 1 << 20})
	key := []byte{0xff, 0x00, 'k'}
	rangeEnd := []byte{0xff, 0x00, 'z'}

	_, err := handlerFor(t, tools, "etcd_get")(t.Context(), map[string]any{
		"key_base64":       base64.StdEncoding.EncodeToString(key),
		"range_end_base64": base64.StdEncoding.EncodeToString(rangeEnd),
		"revision":         "9007199254740993",
		"limit":            "7",
	})
	if err != nil {
		t.Fatalf("etcd_get: %v", err)
	}
	if len(adapter.requests) != 1 {
		t.Fatalf("adapter requests = %d, want 1", len(adapter.requests))
	}
	request := adapter.requests[0]
	if !bytes.Equal(request.Key, key) || !bytes.Equal(request.RangeEnd, rangeEnd) {
		t.Fatalf("request bytes = (%v, %v), want (%v, %v)", request.Key, request.RangeEnd, key, rangeEnd)
	}
	if request.Revision != 9007199254740993 || request.Limit != 7 {
		t.Fatalf("request scalars = (%d, %d), want (9007199254740993, 7)", request.Revision, request.Limit)
	}
}

func TestGetRejectsJSONNumberAndMixedByteInputsBeforeAdapterCall(t *testing.T) {
	adapter := &recordingAdapter{}
	tools := NewToolset(adapter, Config{MaxItems: 100, MaxResponseBytes: 1 << 20})

	for name, params := range map[string]map[string]any{
		"json number": {"key": "key", "limit": json.Number("7")},
		"mixed key": {
			"key": "key", "key_base64": base64.StdEncoding.EncodeToString([]byte("key")),
		},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := handlerFor(t, tools, "etcd_get")(t.Context(), params)
			if err == nil {
				t.Fatal("etcd_get error = nil, want parser rejection")
			}
		})
	}
	if len(adapter.requests) != 0 {
		t.Fatalf("adapter requests = %d, want 0", len(adapter.requests))
	}
}

func TestGetRejectsBase64WithNewlineBeforeAdapterCall(t *testing.T) {
	adapter := &recordingAdapter{}
	tools := NewToolset(adapter, Config{MaxItems: 100, MaxResponseBytes: 1 << 20})

	if _, err := handlerFor(t, tools, "etcd_get")(t.Context(), map[string]any{"key_base64": "a2V5\n"}); err == nil {
		t.Fatal("etcd_get error = nil, want parser rejection")
	}
	if len(adapter.requests) != 0 {
		t.Fatalf("adapter requests = %d, want 0", len(adapter.requests))
	}
}

func TestLeaseTTLAttachedKeysFollowConditionalArrayContract(t *testing.T) {
	keys := [][]byte{}
	withoutKeys, err := encodeResponse(LeaseTTLResponse{}, 1<<20)
	if err != nil {
		t.Fatalf("encode response without keys: %v", err)
	}
	withEmptyKeys, err := encodeResponse(LeaseTTLResponse{Keys: &keys}, 1<<20)
	if err != nil {
		t.Fatalf("encode response with empty keys: %v", err)
	}
	if strings.Contains(withoutKeys, "keys_base64") {
		t.Fatalf("keys must be omitted unless requested: %s", withoutKeys)
	}
	if !strings.Contains(withEmptyKeys, `"keys_base64":[]`) {
		t.Fatalf("requested empty keys must be an array: %s", withEmptyKeys)
	}
}

// Seam: Toolset.GetTools is the canonical approved-tool catalogue used by
// both CLI and MCP. The test observes only the public MCP definitions.
func TestToolsetPublishesOnlyApprovedReadOnlyTools(t *testing.T) {
	tools := NewToolset(nil, Config{})
	want := map[string]struct{}{
		"etcd_get":         {},
		"etcd_lease_ttl":   {},
		"etcd_lease_list":  {},
		"etcd_member_list": {},
		"etcd_alarm_list":  {},
		"etcd_status":      {},
		"etcd_hash_kv":     {},
		"etcd_auth_status": {},
		"etcd_user_get":    {},
		"etcd_user_list":   {},
		"etcd_role_get":    {},
		"etcd_role_list":   {},
	}

	got := make(map[string]struct{}, len(tools.GetTools()))
	for _, serverTool := range tools.GetTools() {
		name := serverTool.Tool.Name
		got[name] = struct{}{}
		if _, ok := want[name]; !ok {
			t.Fatalf("unapproved tool %q is registered", name)
		}
		if serverTool.Domain != "etcd" {
			t.Fatalf("tool %q domain = %q, want etcd", name, serverTool.Domain)
		}

		wire, err := json.Marshal(serverTool.Tool)
		if err != nil {
			t.Fatalf("marshal schema for %q: %v", name, err)
		}
		var decoded struct {
			InputSchema map[string]any `json:"inputSchema"`
		}
		if err := json.Unmarshal(wire, &decoded); err != nil {
			t.Fatalf("unmarshal schema for %q: %v", name, err)
		}
		if additional, ok := decoded.InputSchema["additionalProperties"].(bool); !ok || additional {
			t.Fatalf("tool %q additionalProperties = %#v, want false", name, decoded.InputSchema["additionalProperties"])
		}
		if annotation := serverTool.Tool.Annotations; annotation.ReadOnlyHint == nil || !*annotation.ReadOnlyHint ||
			annotation.DestructiveHint == nil || *annotation.DestructiveHint ||
			annotation.IdempotentHint == nil || !*annotation.IdempotentHint ||
			annotation.OpenWorldHint == nil || !*annotation.OpenWorldHint {
			t.Fatalf("tool %q annotations = %+v, want read-only/idempotent/open-world", name, annotation)
		}
	}

	if len(got) != len(want) {
		t.Fatalf("registered %d tools, want %d: %v", len(got), len(want), got)
	}
	for name := range want {
		if _, ok := got[name]; !ok {
			t.Fatalf("approved tool %q is missing", name)
		}
	}
	for _, forbidden := range []string{"echo", "ping", "etcd_authenticate"} {
		if _, ok := got[forbidden]; ok {
			t.Fatalf("forbidden tool %q is registered", forbidden)
		}
	}
}

// Seam: handler parameter parsing is complete before Adapter.Do. This keeps
// untrusted MCP input from reaching any client method.
func TestGetRejectsUnknownParameterBeforeAdapterCall(t *testing.T) {
	adapter := &recordingAdapter{}
	tools := NewToolset(adapter, Config{MaxItems: 100, MaxResponseBytes: 1 << 20})

	result, err := handlerFor(t, tools, "etcd_get")(t.Context(), map[string]any{
		"key":      "/app/config",
		"surprise": true,
	})
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "unknown") {
		t.Fatalf("unknown parameter error = %v, want unknown parameter rejection", err)
	}
	if result != "" {
		t.Fatalf("unknown parameter result = %q, want no result", result)
	}
	if len(adapter.requests) != 0 {
		t.Fatalf("adapter requests = %d, want 0", len(adapter.requests))
	}
}

// Seam: endpoint validation happens before Adapter.Do so maintenance tools can
// select only an exact configured endpoint, never an arbitrary MCP target.
func TestStatusRejectsUnconfiguredEndpointBeforeAdapterCall(t *testing.T) {
	adapter := &recordingAdapter{}
	tools := NewToolset(adapter, Config{
		Endpoints:        []string{"https://127.0.0.1:2379"},
		MaxItems:         100,
		MaxResponseBytes: 1 << 20,
	})

	result, err := handlerFor(t, tools, "etcd_status")(t.Context(), map[string]any{
		"endpoint": "https://unconfigured.example:2379",
	})
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "endpoint") {
		t.Fatalf("unconfigured endpoint error = %v, want endpoint rejection", err)
	}
	if result != "" {
		t.Fatalf("unconfigured endpoint result = %q, want no result", result)
	}
	if len(adapter.requests) != 0 {
		t.Fatalf("adapter requests = %d, want 0", len(adapter.requests))
	}
}

// Seam: a tool handler encodes a domain GetResponse into MCP text and enforces
// MaxResponseBytes on that emitted JSON. It must fail without partial text.
func TestGetRejectsResponseBeyondConfiguredByteLimit(t *testing.T) {
	adapter := &recordingAdapter{response: GetResponse{
		Header: Header{ClusterID: 1, MemberID: 2, Revision: 3, RaftTerm: 4},
		KVs: []KeyValue{{
			Key:            []byte("/app/config"),
			Value:          []byte(strings.Repeat("v", 128)),
			includeValue:   true,
			CreateRevision: 1,
			ModRevision:    2,
			Version:        3,
			LeaseID:        4,
		}},
		Count: 1,
	}}
	tools := NewToolset(adapter, Config{
		RequestTimeout:   time.Second,
		MaxItems:         100,
		MaxResponseBytes: 64,
	})

	result, err := handlerFor(t, tools, "etcd_get")(t.Context(), map[string]any{
		"key":            "/app/config",
		"include_values": true,
	})
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "response") {
		t.Fatalf("oversize response error = %v, want response byte-limit rejection", err)
	}
	if result != "" {
		t.Fatalf("oversize response result = %q, want no partial JSON", result)
	}
	if len(adapter.requests) != 1 || adapter.requests[0].Method != "get" {
		t.Fatalf("adapter requests = %+v, want one get request", adapter.requests)
	}
	if !adapter.deadlineObserved {
		t.Fatal("get request context had no configured deadline")
	}
}

func TestEveryApprovedToolMapsRequestAndPropagatesAdapterErrors(t *testing.T) {
	tests := []struct {
		tool   string
		params map[string]any
		method string
	}{
		{"etcd_get", map[string]any{"key": "key", "limit": "1"}, methodGet},
		{"etcd_lease_ttl", map[string]any{"lease_id": "1"}, methodLeaseTTL},
		{"etcd_lease_list", map[string]any{}, methodLeaseList},
		{"etcd_member_list", map[string]any{}, methodMemberList},
		{"etcd_alarm_list", map[string]any{}, methodAlarmList},
		{"etcd_status", map[string]any{"endpoint": "https://127.0.0.1:2379"}, methodStatus},
		{"etcd_hash_kv", map[string]any{"endpoint": "https://127.0.0.1:2379", "revision": "1"}, methodHashKV},
		{"etcd_auth_status", map[string]any{}, methodAuthStatus},
		{"etcd_user_get", map[string]any{"user": "reader"}, methodUserGet},
		{"etcd_user_list", map[string]any{}, methodUserList},
		{"etcd_role_get", map[string]any{"role": "reader"}, methodRoleGet},
		{"etcd_role_list", map[string]any{}, methodRoleList},
	}

	for _, tt := range tests {
		t.Run(tt.tool, func(t *testing.T) {
			adapter := &recordingAdapter{response: map[string]any{"ok": true}}
			tools := NewToolset(adapter, Config{Endpoints: []string{"https://127.0.0.1:2379"}, MaxItems: 100, MaxResponseBytes: 1 << 20})
			result, err := handlerFor(t, tools, tt.tool)(t.Context(), tt.params)
			if err != nil {
				t.Fatalf("handler error = %v", err)
			}
			if result != `{"ok":true}` || len(adapter.requests) != 1 || adapter.requests[0].Method != tt.method {
				t.Fatalf("result=%q requests=%+v", result, adapter.requests)
			}

			adapter = &recordingAdapter{err: context.DeadlineExceeded}
			tools = NewToolset(adapter, Config{Endpoints: []string{"https://127.0.0.1:2379"}, MaxItems: 100, MaxResponseBytes: 1 << 20})
			result, err = handlerFor(t, tools, tt.tool)(t.Context(), tt.params)
			if err == nil || result != "" || len(adapter.requests) != 1 {
				t.Fatalf("adapter error result=%q err=%v requests=%+v", result, err, adapter.requests)
			}
		})
	}
}

func TestEveryToolRejectsUnknownParameterBeforeAdapterCall(t *testing.T) {
	params := map[string]map[string]any{
		"etcd_get":         {"key": "key", "unexpected": true},
		"etcd_lease_ttl":   {"lease_id": "1", "unexpected": true},
		"etcd_lease_list":  {"unexpected": true},
		"etcd_member_list": {"unexpected": true},
		"etcd_alarm_list":  {"unexpected": true},
		"etcd_status":      {"endpoint": "https://127.0.0.1:2379", "unexpected": true},
		"etcd_hash_kv":     {"endpoint": "https://127.0.0.1:2379", "unexpected": true},
		"etcd_auth_status": {"unexpected": true},
		"etcd_user_get":    {"user": "reader", "unexpected": true},
		"etcd_user_list":   {"unexpected": true},
		"etcd_role_get":    {"role": "reader", "unexpected": true},
		"etcd_role_list":   {"unexpected": true},
	}

	for tool, toolParams := range params {
		t.Run(tool, func(t *testing.T) {
			adapter := &recordingAdapter{}
			tools := NewToolset(adapter, Config{Endpoints: []string{"https://127.0.0.1:2379"}, MaxItems: 100, MaxResponseBytes: 1 << 20})
			if _, err := handlerFor(t, tools, tool)(t.Context(), toolParams); err == nil {
				t.Fatal("handler error = nil, want unknown parameter rejection")
			}
			if len(adapter.requests) != 0 {
				t.Fatalf("adapter requests = %+v, want none", adapter.requests)
			}
		})
	}
}

func TestSchemasExposeCanonicalByteAndDecimalFields(t *testing.T) {
	tools := NewToolset(nil, Config{})
	get := toolDefinition(t, tools, "etcd_get")
	var getSchema map[string]any
	if err := json.Unmarshal(get.RawInputSchema, &getSchema); err != nil {
		t.Fatalf("unmarshal get schema: %v", err)
	}
	properties := getSchema["properties"].(map[string]any)
	for _, field := range []string{"key", "key_base64", "range_end", "range_end_base64", "revision", "limit"} {
		if _, ok := properties[field]; !ok {
			t.Fatalf("get schema missing %q", field)
		}
	}
	for _, field := range []string{"revision", "limit"} {
		property := properties[field].(map[string]any)
		if property["type"] != "string" || property["pattern"] != signedDecimalPattern {
			t.Fatalf("%s schema = %#v", field, property)
		}
	}
	if _, ok := getSchema["oneOf"]; !ok {
		t.Fatal("get schema is missing key/key_base64 oneOf")
	}
}

func TestEncodeResponseUsesCanonicalEmptyArraysAndDTOFields(t *testing.T) {
	encoded, err := encodeResponse(struct {
		Header  Header       `json:"header"`
		Leases  []Lease      `json:"leases"`
		Members []Member     `json:"members"`
		Errors  []string     `json:"errors"`
		Perms   []Permission `json:"permissions"`
		Alarms  []Alarm      `json:"alarms"`
	}{
		Header: Header{}, Leases: []Lease{}, Members: []Member{}, Errors: []string{}, Perms: []Permission{}, Alarms: []Alarm{},
	}, 1<<20)
	if err != nil {
		t.Fatalf("encodeResponse: %v", err)
	}
	for _, field := range []string{`"leases":[]`, `"members":[]`, `"errors":[]`, `"permissions":[]`, `"alarms":[]`} {
		if !strings.Contains(encoded, field) {
			t.Fatalf("encoded response %q is missing %s", encoded, field)
		}
	}
}

func handlerFor(t *testing.T, tools *Toolset, name string) func(context.Context, map[string]any) (string, error) {
	t.Helper()
	for _, serverTool := range tools.GetTools() {
		if serverTool.Tool.Name == name {
			return serverTool.Handler
		}
	}
	t.Fatalf("tool %q is not registered", name)
	return nil
}

func toolDefinition(t *testing.T, tools *Toolset, name string) mcp.Tool {
	t.Helper()
	for _, serverTool := range tools.GetTools() {
		if serverTool.Tool.Name == name {
			return serverTool.Tool
		}
	}
	t.Fatalf("tool %q is not registered", name)
	return mcp.Tool{}
}

type recordingAdapter struct {
	requests         []Request
	response         any
	err              error
	deadlineObserved bool
}

func (a *recordingAdapter) Do(ctx context.Context, request Request) (any, error) {
	if _, ok := ctx.Deadline(); ok {
		a.deadlineObserved = true
	}
	a.requests = append(a.requests, request)
	return a.response, a.err
}
