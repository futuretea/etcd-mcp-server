package etcd

import (
	"context"
	"fmt"
	"strings"

	etcdserverpb "go.etcd.io/etcd/api/v3/etcdserverpb"
	clientv3 "go.etcd.io/etcd/client/v3"
)

// NewV3Adapter exposes only the approved unary read operations of client/v3.
func NewV3Adapter(client *clientv3.Client) Adapter {
	if client == nil {
		return nil
	}
	return v3Adapter{client: client}
}

type v3Adapter struct {
	client *clientv3.Client
}

func (a v3Adapter) Do(ctx context.Context, request Request) (any, error) {
	switch request.Method {
	case methodGet:
		return a.get(ctx, request)
	case methodLeaseTTL:
		return a.leaseTTL(ctx, request)
	case methodLeaseList:
		return a.leaseList(ctx)
	case methodMemberList:
		return a.memberList(ctx)
	case methodAlarmList:
		return a.alarmList(ctx)
	case methodStatus:
		return a.status(ctx, request)
	case methodHashKV:
		return a.hashKV(ctx, request)
	case methodAuthStatus:
		return a.authStatus(ctx)
	case methodUserGet:
		return a.userGet(ctx, request)
	case methodUserList:
		return a.userList(ctx)
	case methodRoleGet:
		return a.roleGet(ctx, request)
	case methodRoleList:
		return a.roleList(ctx)
	default:
		return nil, fmt.Errorf("unapproved etcd method %q", request.Method)
	}
}

func (a v3Adapter) get(ctx context.Context, request Request) (GetResponse, error) {
	options := []clientv3.OpOption{clientv3.WithLimit(request.Limit)}
	if request.Prefix {
		options = append(options, clientv3.WithPrefix())
	}
	if request.HasRangeEnd {
		options = append(options, clientv3.WithRange(string(request.RangeEnd)))
	}
	if request.Revision != 0 {
		options = append(options, clientv3.WithRev(request.Revision))
	}
	if !request.IncludeValues {
		options = append(options, clientv3.WithKeysOnly())
	}
	response, err := a.client.Get(ctx, string(request.Key), options...)
	if err != nil {
		return GetResponse{}, err
	}
	result := GetResponse{
		Header: header(response.Header), Count: response.Count, More: response.More,
		KVs: make([]KeyValue, 0, len(response.Kvs)),
	}
	for _, kv := range response.Kvs {
		result.KVs = append(result.KVs, KeyValue{
			Key: kv.Key, Value: valueWhenRequested(kv.Value, request.IncludeValues), includeValue: request.IncludeValues,
			CreateRevision: kv.CreateRevision, ModRevision: kv.ModRevision,
			Version: kv.Version, LeaseID: kv.Lease,
		})
	}
	return result, nil
}

func (a v3Adapter) leaseTTL(ctx context.Context, request Request) (LeaseTTLResponse, error) {
	options := []clientv3.LeaseOption{}
	if request.IncludeKeys {
		options = append(options, clientv3.WithAttachedKeys())
	}
	response, err := a.client.TimeToLive(ctx, clientv3.LeaseID(request.LeaseID), options...)
	if err != nil {
		return LeaseTTLResponse{}, err
	}
	result := LeaseTTLResponse{
		Header: header(response.ResponseHeader), LeaseID: int64(response.ID), TTL: response.TTL,
		GrantedTTL: response.GrantedTTL,
	}
	if request.IncludeKeys {
		keys := cloneByteSlices(response.Keys)
		result.Keys = &keys
	}
	return result, nil
}

func (a v3Adapter) leaseList(ctx context.Context) (LeaseListResponse, error) {
	response, err := a.client.Leases(ctx)
	if err != nil {
		return LeaseListResponse{}, err
	}
	result := LeaseListResponse{Header: header(response.ResponseHeader), Leases: make([]Lease, 0, len(response.Leases))}
	for _, lease := range response.Leases {
		result.Leases = append(result.Leases, Lease{LeaseID: int64(lease.ID)})
	}
	return result, nil
}

func (a v3Adapter) memberList(ctx context.Context) (MemberListResponse, error) {
	response, err := a.client.MemberList(ctx)
	if err != nil {
		return MemberListResponse{}, err
	}
	result := MemberListResponse{Header: header(response.Header), Members: make([]Member, 0, len(response.Members))}
	for _, member := range response.Members {
		result.Members = append(result.Members, Member{MemberID: member.ID, Name: member.Name, PeerURLs: cloneStrings(member.PeerURLs), ClientURLs: cloneStrings(member.ClientURLs), IsLearner: member.IsLearner})
	}
	return result, nil
}

func (a v3Adapter) alarmList(ctx context.Context) (AlarmListResponse, error) {
	response, err := a.client.AlarmList(ctx)
	if err != nil {
		return AlarmListResponse{}, err
	}
	result := AlarmListResponse{Header: header(response.Header), Alarms: make([]Alarm, 0, len(response.Alarms))}
	for _, alarm := range response.Alarms {
		result.Alarms = append(result.Alarms, Alarm{MemberID: alarm.MemberID, Alarm: alarmName(alarm.Alarm.String())})
	}
	return result, nil
}

func (a v3Adapter) status(ctx context.Context, request Request) (StatusResponse, error) {
	response, err := a.client.Status(ctx, request.Endpoint)
	if err != nil {
		return StatusResponse{}, err
	}
	return StatusResponse{
		Header: header(response.Header), Version: response.Version, DBSize: response.DbSize,
		LeaderID: response.Leader, RaftIndex: response.RaftIndex, RaftTerm: response.RaftTerm,
		RaftAppliedIndex: response.RaftAppliedIndex, Errors: cloneStrings(response.Errors),
		DBSizeInUse: response.DbSizeInUse, IsLearner: response.IsLearner,
	}, nil
}

func (a v3Adapter) hashKV(ctx context.Context, request Request) (HashKVResponse, error) {
	response, err := a.client.HashKV(ctx, request.Endpoint, request.Revision)
	if err != nil {
		return HashKVResponse{}, err
	}
	return HashKVResponse{Header: header(response.Header), Hash: response.Hash, CompactRevision: response.CompactRevision}, nil
}

func (a v3Adapter) authStatus(ctx context.Context) (AuthStatusResponse, error) {
	response, err := a.client.AuthStatus(ctx)
	if err != nil {
		return AuthStatusResponse{}, err
	}
	return AuthStatusResponse{Header: header(response.Header), Enabled: response.Enabled, AuthRevision: response.AuthRevision}, nil
}

func (a v3Adapter) userGet(ctx context.Context, request Request) (UserGetResponse, error) {
	response, err := a.client.UserGet(ctx, request.User)
	if err != nil {
		return UserGetResponse{}, err
	}
	return UserGetResponse{Header: header(response.Header), Roles: cloneStrings(response.Roles)}, nil
}

func (a v3Adapter) userList(ctx context.Context) (UserListResponse, error) {
	response, err := a.client.UserList(ctx)
	if err != nil {
		return UserListResponse{}, err
	}
	return UserListResponse{Header: header(response.Header), Users: cloneStrings(response.Users)}, nil
}

func (a v3Adapter) roleGet(ctx context.Context, request Request) (RoleGetResponse, error) {
	response, err := a.client.RoleGet(ctx, request.Role)
	if err != nil {
		return RoleGetResponse{}, err
	}
	result := RoleGetResponse{Header: header(response.Header), Permissions: make([]Permission, 0, len(response.Perm))}
	for _, permission := range response.Perm {
		result.Permissions = append(result.Permissions, Permission{Permission: strings.ToLower(permission.PermType.String()), Key: permission.Key, RangeEnd: permission.RangeEnd})
	}
	return result, nil
}

func (a v3Adapter) roleList(ctx context.Context) (RoleListResponse, error) {
	response, err := a.client.RoleList(ctx)
	if err != nil {
		return RoleListResponse{}, err
	}
	return RoleListResponse{Header: header(response.Header), Roles: cloneStrings(response.Roles)}, nil
}

func header(value *etcdserverpb.ResponseHeader) Header {
	if value == nil {
		return Header{}
	}
	return Header{ClusterID: value.ClusterId, MemberID: value.MemberId, Revision: value.Revision, RaftTerm: value.RaftTerm}
}

func valueWhenRequested(value []byte, includeValues bool) []byte {
	if !includeValues {
		return nil
	}
	return value
}

func cloneStrings(values []string) []string {
	return append([]string{}, values...)
}

func cloneByteSlices(values [][]byte) [][]byte {
	return base64ByteSlices(values)
}

func alarmName(value string) string {
	return strings.ToLower(value)
}
