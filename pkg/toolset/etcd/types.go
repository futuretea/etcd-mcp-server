// Package etcd exposes the approved unary read-only etcd MCP toolset.
package etcd

import (
	"context"
	"encoding/json"
	"time"
)

const (
	methodGet        = "get"
	methodLeaseTTL   = "lease_ttl"
	methodLeaseList  = "lease_list"
	methodMemberList = "member_list"
	methodAlarmList  = "alarm_list"
	methodStatus     = "status"
	methodHashKV     = "hash_kv"
	methodAuthStatus = "auth_status"
	methodUserGet    = "user_get"
	methodUserList   = "user_list"
	methodRoleGet    = "role_get"
	methodRoleList   = "role_list"
)

// Config is the non-secret toolset runtime configuration.
type Config struct {
	Endpoints        []string
	RequestTimeout   time.Duration
	MaxItems         int
	MaxResponseBytes int
}

// Request is the parsed, approved etcd operation. It deliberately cannot
// carry client TLS material, credentials, tokens, or arbitrary RPC names.
type Request struct {
	Method        string
	Key           []byte
	Prefix        bool
	RangeEnd      []byte
	HasRangeEnd   bool
	Revision      int64
	Limit         int64
	IncludeValues bool
	LeaseID       int64
	IncludeKeys   bool
	Endpoint      string
	User          string
	Role          string
}

// Adapter is the narrow seam between MCP request validation and client/v3.
// A fake can record only the approved Request without owning a live client.
type Adapter interface {
	Do(context.Context, Request) (any, error)
}

// Header contains etcd metadata. 64-bit values remain strings on JSON output.
type Header struct {
	ClusterID uint64 `json:"cluster_id,string"`
	MemberID  uint64 `json:"member_id,string"`
	Revision  int64  `json:"revision,string"`
	RaftTerm  uint64 `json:"raft_term,string"`
}

// KeyValue is the stable MCP representation of an etcd KV pair.
type KeyValue struct {
	Key            []byte `json:"key_base64"`
	Value          []byte `json:"-"`
	CreateRevision int64  `json:"create_revision,string"`
	ModRevision    int64  `json:"mod_revision,string"`
	Version        int64  `json:"version,string"`
	LeaseID        int64  `json:"lease_id,string"`
	includeValue   bool
}

func (v KeyValue) MarshalJSON() ([]byte, error) {
	type response struct {
		Key            []byte  `json:"key_base64"`
		Value          *[]byte `json:"value_base64,omitempty"`
		CreateRevision int64   `json:"create_revision,string"`
		ModRevision    int64   `json:"mod_revision,string"`
		Version        int64   `json:"version,string"`
		LeaseID        int64   `json:"lease_id,string"`
	}
	result := response{
		Key: base64Bytes(v.Key), CreateRevision: v.CreateRevision, ModRevision: v.ModRevision,
		Version: v.Version, LeaseID: v.LeaseID,
	}
	if v.includeValue || v.Value != nil {
		value := v.Value
		if value == nil {
			value = []byte{}
		}
		result.Value = &value
	}
	return json.Marshal(result)
}

// GetResponse is a stable response for etcd_get.
type GetResponse struct {
	Header Header     `json:"header"`
	KVs    []KeyValue `json:"kvs"`
	Count  int64      `json:"count,string"`
	More   bool       `json:"more"`
}

type LeaseTTLResponse struct {
	Header     Header    `json:"header"`
	LeaseID    int64     `json:"lease_id,string"`
	TTL        int64     `json:"ttl,string"`
	GrantedTTL int64     `json:"granted_ttl,string"`
	Keys       *[][]byte `json:"keys_base64,omitempty"`
}

func (r LeaseTTLResponse) MarshalJSON() ([]byte, error) {
	type response struct {
		Header     Header    `json:"header"`
		LeaseID    int64     `json:"lease_id,string"`
		TTL        int64     `json:"ttl,string"`
		GrantedTTL int64     `json:"granted_ttl,string"`
		Keys       *[][]byte `json:"keys_base64,omitempty"`
	}
	result := response{
		Header: r.Header, LeaseID: r.LeaseID, TTL: r.TTL, GrantedTTL: r.GrantedTTL,
	}
	if r.Keys != nil {
		keys := base64ByteSlices(*r.Keys)
		result.Keys = &keys
	}
	return json.Marshal(result)
}

type LeaseListResponse struct {
	Header Header  `json:"header"`
	Leases []Lease `json:"leases"`
}

type Lease struct {
	LeaseID int64 `json:"lease_id,string"`
}

type Member struct {
	MemberID   uint64   `json:"member_id,string"`
	Name       string   `json:"name"`
	PeerURLs   []string `json:"peer_urls"`
	ClientURLs []string `json:"client_urls"`
	IsLearner  bool     `json:"is_learner"`
}

type MemberListResponse struct {
	Header  Header   `json:"header"`
	Members []Member `json:"members"`
}

type Alarm struct {
	MemberID uint64 `json:"member_id,string"`
	Alarm    string `json:"alarm"`
}

type AlarmListResponse struct {
	Header Header  `json:"header"`
	Alarms []Alarm `json:"alarms"`
}

type StatusResponse struct {
	Header           Header   `json:"header"`
	Version          string   `json:"version"`
	DBSize           int64    `json:"db_size,string"`
	LeaderID         uint64   `json:"leader_id,string"`
	RaftIndex        uint64   `json:"raft_index,string"`
	RaftTerm         uint64   `json:"raft_term,string"`
	RaftAppliedIndex uint64   `json:"raft_applied_index,string"`
	Errors           []string `json:"errors"`
	DBSizeInUse      int64    `json:"db_size_in_use,string"`
	IsLearner        bool     `json:"is_learner"`
}

type HashKVResponse struct {
	Header          Header `json:"header"`
	Hash            uint32 `json:"hash"`
	CompactRevision int64  `json:"compact_revision,string"`
}

type AuthStatusResponse struct {
	Header       Header `json:"header"`
	Enabled      bool   `json:"enabled"`
	AuthRevision uint64 `json:"auth_revision,string"`
}

type UserGetResponse struct {
	Header Header   `json:"header"`
	Roles  []string `json:"roles"`
}

type UserListResponse struct {
	Header Header   `json:"header"`
	Users  []string `json:"users"`
}

type Permission struct {
	Permission string `json:"permission"`
	Key        []byte `json:"key_base64"`
	RangeEnd   []byte `json:"range_end_base64"`
}

func (p Permission) MarshalJSON() ([]byte, error) {
	type response struct {
		Permission string `json:"permission"`
		Key        []byte `json:"key_base64"`
		RangeEnd   []byte `json:"range_end_base64"`
	}
	return json.Marshal(response{
		Permission: p.Permission,
		Key:        base64Bytes(p.Key),
		RangeEnd:   base64Bytes(p.RangeEnd),
	})
}

func base64Bytes(value []byte) []byte {
	if value == nil {
		return []byte{}
	}
	return value
}

func base64ByteSlices(values [][]byte) [][]byte {
	normalized := make([][]byte, len(values))
	for index, value := range values {
		normalized[index] = base64Bytes(value)
	}
	return normalized
}

type RoleGetResponse struct {
	Header      Header       `json:"header"`
	Permissions []Permission `json:"permissions"`
}

type RoleListResponse struct {
	Header Header   `json:"header"`
	Roles  []string `json:"roles"`
}
