package etcd

import (
	"encoding/base64"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestLeaseTTLAcceptsCanonicalSignedLeaseIDs(t *testing.T) {
	for _, leaseID := range []string{"-1", "-0", "0", "1"} {
		t.Run(leaseID, func(t *testing.T) {
			adapter := &recordingAdapter{response: LeaseTTLResponse{}}
			tools := NewToolset(adapter, Config{MaxItems: 100, MaxResponseBytes: 1 << 20})
			if _, err := handlerFor(t, tools, "etcd_lease_ttl")(t.Context(), map[string]any{"lease_id": leaseID}); err != nil {
				t.Fatalf("etcd_lease_ttl(%q): %v", leaseID, err)
			}
			if len(adapter.requests) != 1 {
				t.Fatalf("adapter requests = %d, want 1", len(adapter.requests))
			}
		})
	}
}

func TestEveryToolPublishesExactApprovedSchemaFields(t *testing.T) {
	expected := map[string][]string{
		"etcd_get":         {"key", "key_base64", "prefix", "range_end", "range_end_base64", "revision", "limit", "include_values"},
		"etcd_lease_ttl":   {"lease_id", "include_attached_keys"},
		"etcd_lease_list":  {},
		"etcd_member_list": {},
		"etcd_alarm_list":  {},
		"etcd_status":      {"endpoint"},
		"etcd_hash_kv":     {"endpoint", "revision"},
		"etcd_auth_status": {},
		"etcd_user_get":    {"user"},
		"etcd_user_list":   {},
		"etcd_role_get":    {"role"},
		"etcd_role_list":   {},
	}
	required := map[string][]string{
		"etcd_get":         nil,
		"etcd_lease_ttl":   {"lease_id"},
		"etcd_lease_list":  nil,
		"etcd_member_list": nil,
		"etcd_alarm_list":  nil,
		"etcd_status":      {"endpoint"},
		"etcd_hash_kv":     {"endpoint"},
		"etcd_auth_status": nil,
		"etcd_user_get":    {"user"},
		"etcd_user_list":   nil,
		"etcd_role_get":    {"role"},
		"etcd_role_list":   nil,
	}
	decimalFields := map[string][]string{
		"etcd_get":       {"revision", "limit"},
		"etcd_lease_ttl": {"lease_id"},
		"etcd_hash_kv":   {"revision"},
	}

	tools := NewToolset(nil, Config{})
	for name, wantFields := range expected {
		t.Run(name, func(t *testing.T) {
			definition := toolDefinition(t, tools, name)
			var schema struct {
				AdditionalProperties bool           `json:"additionalProperties"`
				Properties           map[string]any `json:"properties"`
				Required             []string       `json:"required"`
			}
			if err := json.Unmarshal(definition.RawInputSchema, &schema); err != nil {
				t.Fatalf("decode schema: %v", err)
			}
			if schema.AdditionalProperties || len(schema.Properties) != len(wantFields) {
				t.Fatalf("schema = %+v, want exactly %d fields and no additional properties", schema, len(wantFields))
			}
			for _, field := range wantFields {
				if _, ok := schema.Properties[field]; !ok {
					t.Fatalf("schema properties = %v, missing %q", schema.Properties, field)
				}
			}
			if strings.Join(schema.Required, ",") != strings.Join(required[name], ",") {
				t.Fatalf("schema required = %v, want %v", schema.Required, required[name])
			}
			for _, field := range decimalFields[name] {
				property := schema.Properties[field].(map[string]any)
				if property["type"] != "string" || property["pattern"] != signedDecimalPattern {
					t.Fatalf("schema %s.%s = %#v", name, field, property)
				}
			}
		})
	}
}

func TestEveryToolEncodesApprovedDTOFixture(t *testing.T) {
	header := Header{ClusterID: ^uint64(0), MemberID: ^uint64(0) - 1, Revision: -1 << 63, RaftTerm: ^uint64(0)}
	headerJSON := func() map[string]any {
		return map[string]any{
			"cluster_id": "18446744073709551615",
			"member_id":  "18446744073709551614",
			"revision":   "-9223372036854775808",
			"raft_term":  "18446744073709551615",
		}
	}
	responseJSON := func(fields map[string]any) map[string]any {
		fields["header"] = headerJSON()
		return fields
	}
	keys := [][]byte{{0xff, 0x00}}
	tests := []struct {
		name     string
		params   map[string]any
		response any
		want     map[string]any
	}{
		{"etcd_get", map[string]any{"key": "key", "include_values": true}, GetResponse{Header: header, KVs: []KeyValue{{Key: []byte{0xff, 0x00}, Value: []byte{0x00, 0xff}, includeValue: true, CreateRevision: -1, ModRevision: 2, Version: 3, LeaseID: 4}}, Count: 1, More: true}, responseJSON(map[string]any{"kvs": []any{map[string]any{"key_base64": "/wA=", "value_base64": "AP8=", "create_revision": "-1", "mod_revision": "2", "version": "3", "lease_id": "4"}}, "count": "1", "more": true})},
		{"etcd_lease_ttl", map[string]any{"lease_id": "-1", "include_attached_keys": true}, LeaseTTLResponse{Header: header, LeaseID: -1, TTL: 0, GrantedTTL: 1, Keys: &keys}, responseJSON(map[string]any{"lease_id": "-1", "ttl": "0", "granted_ttl": "1", "keys_base64": []any{"/wA="}})},
		{"etcd_lease_list", map[string]any{}, LeaseListResponse{Header: header, Leases: []Lease{{LeaseID: -1}}}, responseJSON(map[string]any{"leases": []any{map[string]any{"lease_id": "-1"}}})},
		{"etcd_member_list", map[string]any{}, MemberListResponse{Header: header, Members: []Member{{MemberID: ^uint64(0), Name: "member", PeerURLs: []string{"https://peer"}, ClientURLs: []string{"https://client"}, IsLearner: true}}}, responseJSON(map[string]any{"members": []any{map[string]any{"member_id": "18446744073709551615", "name": "member", "peer_urls": []any{"https://peer"}, "client_urls": []any{"https://client"}, "is_learner": true}}})},
		{"etcd_alarm_list", map[string]any{}, AlarmListResponse{Header: header, Alarms: []Alarm{{MemberID: ^uint64(0), Alarm: "nospace"}}}, responseJSON(map[string]any{"alarms": []any{map[string]any{"member_id": "18446744073709551615", "alarm": "nospace"}}})},
		{"etcd_status", map[string]any{"endpoint": "https://127.0.0.1:2379"}, StatusResponse{Header: header, Version: "3.5.33", DBSize: -1, LeaderID: ^uint64(0), RaftIndex: ^uint64(0), RaftTerm: ^uint64(0), RaftAppliedIndex: ^uint64(0), Errors: []string{"warning"}, DBSizeInUse: 1, IsLearner: true}, responseJSON(map[string]any{"version": "3.5.33", "db_size": "-1", "leader_id": "18446744073709551615", "raft_index": "18446744073709551615", "raft_term": "18446744073709551615", "raft_applied_index": "18446744073709551615", "errors": []any{"warning"}, "db_size_in_use": "1", "is_learner": true})},
		{"etcd_hash_kv", map[string]any{"endpoint": "https://127.0.0.1:2379", "revision": "-1"}, HashKVResponse{Header: header, Hash: 7, CompactRevision: -1}, responseJSON(map[string]any{"hash": 7, "compact_revision": "-1"})},
		{"etcd_auth_status", map[string]any{}, AuthStatusResponse{Header: header, Enabled: true, AuthRevision: ^uint64(0)}, responseJSON(map[string]any{"enabled": true, "auth_revision": "18446744073709551615"})},
		{"etcd_user_get", map[string]any{"user": "reader"}, UserGetResponse{Header: header, Roles: []string{"reader"}}, responseJSON(map[string]any{"roles": []any{"reader"}})},
		{"etcd_user_list", map[string]any{}, UserListResponse{Header: header, Users: []string{"reader"}}, responseJSON(map[string]any{"users": []any{"reader"}})},
		{"etcd_role_get", map[string]any{"role": "reader"}, RoleGetResponse{Header: header, Permissions: []Permission{{Permission: "readwrite", Key: []byte{0xff}, RangeEnd: []byte{0x00}}}}, responseJSON(map[string]any{"permissions": []any{map[string]any{"permission": "readwrite", "key_base64": "/w==", "range_end_base64": "AA=="}}})},
		{"etcd_role_list", map[string]any{}, RoleListResponse{Header: header, Roles: []string{"reader"}}, responseJSON(map[string]any{"roles": []any{"reader"}})},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adapter := &recordingAdapter{response: tt.response}
			tools := NewToolset(adapter, Config{Endpoints: []string{"https://127.0.0.1:2379"}, MaxItems: 100, MaxResponseBytes: 1 << 20})
			encoded, err := handlerFor(t, tools, tt.name)(t.Context(), tt.params)
			if err != nil {
				t.Fatalf("handler: %v", err)
			}
			assertJSONEquivalent(t, encoded, tt.want)
		})
	}
}

func TestGetLimitAndByteContractsBeforeAdapter(t *testing.T) {
	adapter := &recordingAdapter{response: GetResponse{KVs: []KeyValue{}}}
	tools := NewToolset(adapter, Config{MaxItems: 3, MaxResponseBytes: 1 << 20})

	if _, err := handlerFor(t, tools, "etcd_get")(t.Context(), map[string]any{"key": "key"}); err != nil {
		t.Fatalf("default limit: %v", err)
	}
	if adapter.requests[0].Limit != 3 {
		t.Fatalf("default limit = %d, want 3", adapter.requests[0].Limit)
	}

	for _, limit := range []string{"0", "-1", "1.5", "4"} {
		t.Run("invalid limit "+limit, func(t *testing.T) {
			adapter.requests = nil
			if _, err := handlerFor(t, tools, "etcd_get")(t.Context(), map[string]any{"key": "key", "limit": limit}); err == nil {
				t.Fatal("etcd_get error = nil, want parser rejection")
			}
			if len(adapter.requests) != 0 {
				t.Fatalf("adapter requests = %d, want 0", len(adapter.requests))
			}
		})
	}

	for _, revision := range []string{"-9223372036854775808", "9223372036854775807"} {
		t.Run("int64 revision "+revision, func(t *testing.T) {
			adapter.requests = nil
			if _, err := handlerFor(t, tools, "etcd_get")(t.Context(), map[string]any{"key": "key", "revision": revision}); err != nil {
				t.Fatalf("etcd_get: %v", err)
			}
			if len(adapter.requests) != 1 {
				t.Fatalf("adapter requests = %d, want 1", len(adapter.requests))
			}
		})
	}

	for _, revision := range []string{"9223372036854775808", "-9223372036854775809"} {
		t.Run("out of range revision "+revision, func(t *testing.T) {
			adapter.requests = nil
			if _, err := handlerFor(t, tools, "etcd_get")(t.Context(), map[string]any{"key": "key", "revision": revision}); err == nil {
				t.Fatal("etcd_get error = nil, want parser rejection")
			}
			if len(adapter.requests) != 0 {
				t.Fatalf("adapter requests = %d, want 0", len(adapter.requests))
			}
		})
	}
}

func TestGetByteOutputRoundTripAndValueOmission(t *testing.T) {
	key, value := []byte{0xff, 0x00, 'k'}, []byte{0xff, 0x00, 'v'}
	for _, tt := range []struct {
		name          string
		includeValues bool
		wantValue     bool
	}{
		{name: "without values", includeValues: false, wantValue: false},
		{name: "with values", includeValues: true, wantValue: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			adapter := &recordingAdapter{response: GetResponse{KVs: []KeyValue{{Key: key, Value: valueWhenRequested(value, tt.includeValues), includeValue: tt.includeValues}}}}
			tools := NewToolset(adapter, Config{MaxItems: 100, MaxResponseBytes: 1 << 20})
			encoded, err := handlerFor(t, tools, "etcd_get")(t.Context(), map[string]any{"key_base64": base64.StdEncoding.EncodeToString(key), "include_values": tt.includeValues})
			if err != nil {
				t.Fatalf("etcd_get: %v", err)
			}
			if !strings.Contains(encoded, `"key_base64":"/wBr"`) {
				t.Fatalf("missing Base64 key: %s", encoded)
			}
			hasValue := strings.Contains(encoded, `"value_base64":"/wB2"`)
			if hasValue != tt.wantValue {
				t.Fatalf("value presence = %t, want %t: %s", hasValue, tt.wantValue, encoded)
			}
		})
	}
}

func TestGetPreservesRequestedEmptyValue(t *testing.T) {
	for _, tt := range []struct {
		name         string
		value        []byte
		includeValue bool
		wantPresent  bool
	}{
		{name: "requested empty slice", value: []byte{}, includeValue: true, wantPresent: true},
		{name: "requested nil slice", value: nil, includeValue: true, wantPresent: true},
		{name: "not requested", value: []byte{}, includeValue: false, wantPresent: false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			adapter := &recordingAdapter{response: GetResponse{KVs: []KeyValue{{Key: []byte("key"), Value: valueWhenRequested(tt.value, tt.includeValue), includeValue: tt.includeValue}}}}
			tools := NewToolset(adapter, Config{MaxItems: 100, MaxResponseBytes: 1 << 20})
			encoded, err := handlerFor(t, tools, "etcd_get")(t.Context(), map[string]any{"key": "key", "include_values": tt.includeValue})
			if err != nil {
				t.Fatalf("etcd_get: %v", err)
			}
			present := strings.Contains(encoded, `"value_base64":""`)
			if present != tt.wantPresent {
				t.Fatalf("empty value presence = %t, want %t: %s", present, tt.wantPresent, encoded)
			}
		})
	}
}

func TestRolePermissionEncodesNilByteFieldsAsBase64Strings(t *testing.T) {
	adapter := &recordingAdapter{response: RoleGetResponse{Permissions: []Permission{{Permission: "read", Key: nil, RangeEnd: nil}}}}
	tools := NewToolset(adapter, Config{MaxItems: 100, MaxResponseBytes: 1 << 20})

	encoded, err := handlerFor(t, tools, "etcd_role_get")(t.Context(), map[string]any{"role": "reader"})
	if err != nil {
		t.Fatalf("etcd_role_get: %v", err)
	}
	for _, field := range []string{`"key_base64":""`, `"range_end_base64":""`} {
		if !strings.Contains(encoded, field) {
			t.Fatalf("nil permission byte field is not Base64 string: %s", encoded)
		}
	}
}

func TestCloneByteSlicesEncodesNilItemsAsBase64Strings(t *testing.T) {
	keys := cloneByteSlices([][]byte{nil})
	encoded, err := json.Marshal(struct {
		Keys [][]byte `json:"keys_base64"`
	}{Keys: keys})
	if err != nil {
		t.Fatalf("marshal attached keys: %v", err)
	}
	if !strings.Contains(string(encoded), `"keys_base64":[""]`) {
		t.Fatalf("nil attached key is not Base64 string: %s", encoded)
	}
}

func assertJSONEquivalent(t *testing.T, encoded string, want map[string]any) {
	t.Helper()
	var got any
	if err := json.Unmarshal([]byte(encoded), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	wantJSON, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("encode expected response: %v", err)
	}
	var expected any
	if err := json.Unmarshal(wantJSON, &expected); err != nil {
		t.Fatalf("decode expected response: %v", err)
	}
	if !reflect.DeepEqual(got, expected) {
		t.Fatalf("response = %s, want %s", encoded, wantJSON)
	}
}
