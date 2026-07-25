package etcd_test

import (
	"encoding/json"
	"testing"

	"github.com/futuretea/etcd-mcp-server/pkg/toolset/etcd"
)

func TestKeyValueSuppliedValueRetainsBase64JSONEncoding(t *testing.T) {
	for _, tt := range []struct {
		name  string
		value []byte
		want  string
	}{
		{name: "non-empty", value: []byte("v"), want: "dg=="},
		{name: "empty", value: []byte{}, want: ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			encoded, err := json.Marshal(etcd.KeyValue{Key: []byte("key"), Value: tt.value})
			if err != nil {
				t.Fatalf("marshal KeyValue: %v", err)
			}
			var result map[string]any
			if err := json.Unmarshal(encoded, &result); err != nil {
				t.Fatalf("unmarshal KeyValue: %v", err)
			}
			if result["value_base64"] != tt.want {
				t.Fatalf("value_base64 = %#v, want %q", result["value_base64"], tt.want)
			}
		})
	}
}

func TestKeyValueNilKeyEncodesAsEmptyBase64String(t *testing.T) {
	encoded, err := json.Marshal(etcd.KeyValue{Key: nil})
	if err != nil {
		t.Fatalf("marshal KeyValue: %v", err)
	}
	var result map[string]any
	if err := json.Unmarshal(encoded, &result); err != nil {
		t.Fatalf("unmarshal KeyValue: %v", err)
	}
	if result["key_base64"] != "" {
		t.Fatalf("key_base64 = %#v, want empty string", result["key_base64"])
	}
}

func TestLeaseTTLRequestedKeysEncodeAsBase64Array(t *testing.T) {
	for _, tt := range []struct {
		name     string
		response etcd.LeaseTTLResponse
		want     any
		present  bool
	}{
		{
			name: "nil key",
			response: func() etcd.LeaseTTLResponse {
				keys := [][]byte{nil}
				return etcd.LeaseTTLResponse{Keys: &keys}
			}(),
			want:    []any{""},
			present: true,
		},
		{
			name: "nil key slice",
			response: func() etcd.LeaseTTLResponse {
				var keys [][]byte
				return etcd.LeaseTTLResponse{Keys: &keys}
			}(),
			want:    []any{},
			present: true,
		},
		{name: "keys not requested", present: false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			encoded, err := json.Marshal(tt.response)
			if err != nil {
				t.Fatalf("marshal LeaseTTLResponse: %v", err)
			}
			var result map[string]any
			if err := json.Unmarshal(encoded, &result); err != nil {
				t.Fatalf("unmarshal LeaseTTLResponse: %v", err)
			}
			got, ok := result["keys_base64"]
			if ok != tt.present {
				t.Fatalf("keys_base64 present = %t, want %t", ok, tt.present)
			}
			if tt.present && !equalJSONValue(got, tt.want) {
				t.Fatalf("keys_base64 = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func equalJSONValue(got, want any) bool {
	gotJSON, err := json.Marshal(got)
	if err != nil {
		return false
	}
	wantJSON, err := json.Marshal(want)
	if err != nil {
		return false
	}
	return string(gotJSON) == string(wantJSON)
}
