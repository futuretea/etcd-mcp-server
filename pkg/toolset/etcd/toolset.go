package etcd

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/futuretea/etcd-mcp-server/pkg/toolset"
)

// Toolset publishes the complete approved read-only tool catalogue.
type Toolset struct {
	adapter Adapter
	config  Config
	tools   []toolset.ServerTool
}

// NewToolset constructs schemas without requiring a runtime client. A nil
// adapter is therefore safe for list and describe commands.
func NewToolset(adapter Adapter, cfg Config) *Toolset {
	if cfg.MaxItems == 0 {
		cfg.MaxItems = 100
	}
	if cfg.MaxResponseBytes == 0 {
		cfg.MaxResponseBytes = 1 << 20
	}
	t := &Toolset{adapter: adapter, config: cfg}
	t.tools = []toolset.ServerTool{
		t.newTool("etcd_get", methodGet, "Read one key or a bounded key range.", nil, propertyMap("key", "string", "UTF-8 key", "key_base64", "string", "RFC 4648 padded Base64 key", "prefix", "boolean", "Read matching prefix", "range_end", "string", "Exclusive UTF-8 range end", "range_end_base64", "string", "RFC 4648 padded Base64 range end", "revision", "string", "Historical int64 revision", "limit", "string", "Maximum returned keys", "include_values", "boolean", "Include Base64 values")),
		t.newTool("etcd_lease_ttl", methodLeaseTTL, "Read lease time-to-live metadata.", []string{"lease_id"}, propertyMap("lease_id", "string", "Decimal lease ID", "include_attached_keys", "boolean", "Include attached key names")),
		t.newTool("etcd_lease_list", methodLeaseList, "List active lease IDs.", nil, nil),
		t.newTool("etcd_member_list", methodMemberList, "List current cluster members.", nil, nil),
		t.newTool("etcd_alarm_list", methodAlarmList, "List active maintenance alarms.", nil, nil),
		t.newTool("etcd_status", methodStatus, "Read the status of a configured endpoint.", []string{"endpoint"}, propertyMap("endpoint", "string", "Exact configured endpoint")),
		t.newTool("etcd_hash_kv", methodHashKV, "Read a KV hash from a configured endpoint.", []string{"endpoint"}, propertyMap("endpoint", "string", "Exact configured endpoint", "revision", "string", "Optional historical int64 revision")),
		t.newTool("etcd_auth_status", methodAuthStatus, "Read whether etcd auth is enabled.", nil, nil),
		t.newTool("etcd_user_get", methodUserGet, "Read a user's roles.", []string{"user"}, propertyMap("user", "string", "User name")),
		t.newTool("etcd_user_list", methodUserList, "List users visible to the configured client.", nil, nil),
		t.newTool("etcd_role_get", methodRoleGet, "Read a role's permissions.", []string{"role"}, propertyMap("role", "string", "Role name")),
		t.newTool("etcd_role_list", methodRoleList, "List roles visible to the configured client.", nil, nil),
	}
	return t
}

func (t *Toolset) GetName() string { return "etcd" }

func (t *Toolset) GetDescription() string { return "Bounded read-only etcd v3.5.33 tools" }

func (t *Toolset) GetTools() []toolset.ServerTool {
	return append([]toolset.ServerTool(nil), t.tools...)
}

func (t *Toolset) newTool(name, method, description string, required []string, properties map[string]any) toolset.ServerTool {
	schema, _ := json.Marshal(schemaFor(method, required, properties))
	definition := mcp.NewToolWithRawSchema(name, description, schema)
	definition.Annotations = mcp.ToolAnnotation{
		ReadOnlyHint: mcp.ToBoolPtr(true), DestructiveHint: mcp.ToBoolPtr(false),
		IdempotentHint: mcp.ToBoolPtr(true), OpenWorldHint: mcp.ToBoolPtr(true),
	}
	return toolset.ServerTool{
		Tool:    definition,
		Handler: t.handler(method), Domain: "etcd",
	}
}

func schemaFor(method string, required []string, properties map[string]any) map[string]any {
	schema := map[string]any{"type": "object", "additionalProperties": false, "properties": properties, "required": required}
	for _, field := range decimalFields(method) {
		properties[field] = map[string]any{"type": "string", "pattern": signedDecimalPattern, "description": properties[field].(map[string]any)["description"]}
	}
	if method == methodGet {
		schema["oneOf"] = []any{
			map[string]any{"required": []string{"key"}},
			map[string]any{"required": []string{"key_base64"}},
		}
		schema["allOf"] = []any{
			map[string]any{"not": map[string]any{"required": []string{"key", "key_base64"}}},
			map[string]any{"not": map[string]any{"required": []string{"range_end", "range_end_base64"}}},
		}
	}
	return schema
}

const signedDecimalPattern = "^-?(0|[1-9][0-9]*)$"

func decimalFields(method string) []string {
	switch method {
	case methodGet:
		return []string{"revision", "limit"}
	case methodLeaseTTL:
		return []string{"lease_id"}
	case methodHashKV:
		return []string{"revision"}
	default:
		return nil
	}
}

func (t *Toolset) handler(method string) toolset.ToolHandler {
	return func(ctx context.Context, params map[string]any) (string, error) {
		request, err := t.parseRequest(method, params)
		if err != nil {
			return "", err
		}
		if t.adapter == nil {
			return "", fmt.Errorf("etcd client is not configured")
		}
		if t.config.RequestTimeout > 0 {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, t.config.RequestTimeout)
			defer cancel()
		}
		response, err := t.adapter.Do(ctx, request)
		if err != nil {
			return "", err
		}
		return encodeResponse(response, t.config.MaxResponseBytes)
	}
}

func (t *Toolset) parseRequest(method string, params map[string]any) (Request, error) {
	if params == nil {
		params = map[string]any{}
	}
	if err := rejectUnknown(params, allowedParameters(method)); err != nil {
		return Request{}, err
	}
	request := Request{Method: method}
	switch method {
	case methodGet:
		return t.parseGet(params, request)
	case methodLeaseTTL:
		return parseLeaseTTL(params, request)
	case methodStatus, methodHashKV:
		return t.parseEndpoint(params, request)
	case methodUserGet:
		return parseNamed(params, request, "user")
	case methodRoleGet:
		return parseNamed(params, request, "role")
	default:
		return request, nil
	}
}

func (t *Toolset) parseGet(params map[string]any, request Request) (Request, error) {
	key, _, err := bytePair(params, "key", "key_base64", true)
	if err != nil {
		return Request{}, err
	}
	request.Key = key
	request.Prefix, err = optionalBool(params, "prefix")
	if err != nil {
		return Request{}, err
	}
	if request.RangeEnd, request.HasRangeEnd, err = bytePair(params, "range_end", "range_end_base64", false); err != nil {
		return Request{}, err
	}
	if request.Prefix && request.HasRangeEnd {
		return Request{}, fmt.Errorf("prefix and range_end are mutually exclusive")
	}
	if request.Revision, err = optionalSignedInt64(params, "revision"); err != nil {
		return Request{}, err
	}
	if request.Limit, err = optionalPositiveInt64(params, "limit"); err != nil {
		return Request{}, err
	}
	if request.Limit == 0 {
		request.Limit = int64(t.config.MaxItems)
	}
	if request.Limit <= 0 || request.Limit > int64(t.config.MaxItems) {
		return Request{}, fmt.Errorf("limit must be between 1 and %d", t.config.MaxItems)
	}
	request.IncludeValues, err = optionalBool(params, "include_values")
	if err != nil {
		return Request{}, err
	}
	return request, nil
}

func (t *Toolset) parseEndpoint(params map[string]any, request Request) (Request, error) {
	endpoint, err := requiredString(params, "endpoint")
	if err != nil {
		return Request{}, err
	}
	if !configuredEndpoint(t.config.Endpoints, endpoint) {
		return Request{}, fmt.Errorf("endpoint %q is not configured", endpoint)
	}
	request.Endpoint = endpoint
	if request.Method == methodHashKV {
		request.Revision, err = optionalSignedInt64(params, "revision")
		if err != nil {
			return Request{}, err
		}
	}
	return request, nil
}

func parseLeaseTTL(params map[string]any, request Request) (Request, error) {
	leaseID, err := requiredSignedInt64(params, "lease_id")
	if err != nil {
		return Request{}, err
	}
	request.LeaseID = leaseID
	request.IncludeKeys, err = optionalBool(params, "include_attached_keys")
	return request, err
}

func parseNamed(params map[string]any, request Request, field string) (Request, error) {
	value, err := requiredString(params, field)
	if err != nil {
		return Request{}, err
	}
	if field == "user" {
		request.User = value
	} else {
		request.Role = value
	}
	return request, nil
}

func encodeResponse(response any, limit int) (string, error) {
	encoded, err := json.Marshal(response)
	if err != nil {
		return "", fmt.Errorf("encode etcd response: %w", err)
	}
	if len(encoded) > limit {
		return "", fmt.Errorf("etcd response exceeds configured byte limit of %d", limit)
	}
	return string(encoded), nil
}

func allowedParameters(method string) map[string]struct{} {
	fields := map[string][]string{
		methodGet: {"key", "key_base64", "prefix", "range_end", "range_end_base64", "revision", "limit", "include_values"}, methodLeaseTTL: {"lease_id", "include_attached_keys"},
		methodStatus: {"endpoint"}, methodHashKV: {"endpoint", "revision"}, methodUserGet: {"user"}, methodRoleGet: {"role"},
	}
	allowed := map[string]struct{}{}
	for _, field := range fields[method] {
		allowed[field] = struct{}{}
	}
	return allowed
}

func rejectUnknown(params map[string]any, allowed map[string]struct{}) error {
	for field := range params {
		if _, ok := allowed[field]; !ok {
			return fmt.Errorf("unknown parameter %q", field)
		}
	}
	return nil
}

func requiredString(params map[string]any, field string) (string, error) {
	value, ok := params[field]
	if !ok {
		return "", fmt.Errorf("%s is required", field)
	}
	text, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("%s must be a string", field)
	}
	if text == "" || !utf8.ValidString(text) {
		return "", fmt.Errorf("%s must be a non-empty UTF-8 string", field)
	}
	return text, nil
}

func optionalBool(params map[string]any, field string) (bool, error) {
	value, ok := params[field]
	if !ok {
		return false, nil
	}
	parsed, ok := value.(bool)
	if !ok {
		return false, fmt.Errorf("%s must be a boolean", field)
	}
	return parsed, nil
}

func bytePair(params map[string]any, textField, base64Field string, required bool) ([]byte, bool, error) {
	textValue, hasText := params[textField]
	base64Value, hasBase64 := params[base64Field]
	if hasText && hasBase64 {
		return nil, false, fmt.Errorf("%s and %s are mutually exclusive", textField, base64Field)
	}
	if !hasText && !hasBase64 {
		if required {
			return nil, false, fmt.Errorf("%s or %s is required", textField, base64Field)
		}
		return nil, false, nil
	}
	if hasText {
		text, ok := textValue.(string)
		if !ok || !utf8.ValidString(text) {
			return nil, false, fmt.Errorf("%s must be a UTF-8 string", textField)
		}
		return []byte(text), true, nil
	}
	encoded, ok := base64Value.(string)
	if !ok {
		return nil, false, fmt.Errorf("%s must be a Base64 string", base64Field)
	}
	if strings.ContainsAny(encoded, "\r\n") {
		return nil, false, fmt.Errorf("%s must be RFC 4648 padded Base64", base64Field)
	}
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, false, fmt.Errorf("decode %s: %w", base64Field, err)
	}
	return decoded, true, nil
}

func requiredSignedInt64(params map[string]any, field string) (int64, error) {
	value, ok := params[field]
	if !ok {
		return 0, fmt.Errorf("%s is required", field)
	}
	return signedInt64(value, field)
}

func optionalPositiveInt64(params map[string]any, field string) (int64, error) {
	value, ok := params[field]
	if !ok {
		return 0, nil
	}
	return positiveInt64(value, field)
}

func positiveInt64(value any, field string) (int64, error) {
	parsed, err := signedInt64(value, field)
	if err != nil || parsed <= 0 {
		if err != nil {
			return 0, err
		}
		return 0, fmt.Errorf("%s must be positive", field)
	}
	return parsed, nil
}

func optionalSignedInt64(params map[string]any, field string) (int64, error) {
	value, ok := params[field]
	if !ok {
		return 0, nil
	}
	return signedInt64(value, field)
}

func signedInt64(value any, field string) (int64, error) {
	text, ok := value.(string)
	if !ok || !canonicalSignedDecimal(text) {
		return 0, fmt.Errorf("%s must be a canonical int64 decimal string", field)
	}
	parsed, err := strconv.ParseInt(text, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%s must be an int64 decimal string", field)
	}
	return parsed, nil
}

func canonicalSignedDecimal(text string) bool {
	if text == "" {
		return false
	}
	if text[0] == '-' {
		text = text[1:]
	}
	if text == "0" {
		return true
	}
	if len(text) == 0 || text[0] == '0' {
		return false
	}
	for _, character := range text {
		if character < '0' || character > '9' {
			return false
		}
	}
	return true
}

func configuredEndpoint(endpoints []string, endpoint string) bool {
	for _, configured := range endpoints {
		if configured == endpoint {
			return true
		}
	}
	return false
}

func propertyMap(values ...string) map[string]any {
	if len(values) == 0 {
		return map[string]any{}
	}
	properties := map[string]any{}
	for index := 0; index < len(values); index += 3 {
		properties[values[index]] = map[string]any{"type": values[index+1], "description": values[index+2]}
	}
	return properties
}
