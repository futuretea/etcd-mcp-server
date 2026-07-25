# etcd MCP Server

`etcd-mcp-server` is an MCP server for the immediate, read-only etcd v3.5.33 operations. It keeps the existing stdio, Streamable HTTP, and SSE transports while exposing only bounded JSON text tools.

## Read-only surface

`etcd_get`, `etcd_lease_ttl`, `etcd_lease_list`, `etcd_member_list`, `etcd_alarm_list`, `etcd_status`, `etcd_hash_kv`, `etcd_auth_status`, `etcd_user_get`, `etcd_user_list`, `etcd_role_get`, and `etcd_role_list` are the complete public tool set.

There are no write, delete, transaction, authentication-management, Watch, Snapshot, RangeStream, reader, or channel tools. `etcd_status` and `etcd_hash_kv` accept only an exact member of `etcd_endpoints`; a tool parameter can never introduce a new dial target.

## Configure and run

Copy [config.example.yaml](config.example.yaml) and set the configured etcd endpoints and optional TLS file paths. The default request boundary is a 5s dial timeout, 10s per-tool timeout, 100 KV items, and 1 MiB JSON text.

```bash
make build
./bin/etcd-mcp-server tools list --json
./bin/etcd-mcp-server --config config.example.yaml tools call etcd_get --params '{"key":"/applications/","prefix":true}'
./bin/etcd-mcp-server --config config.example.yaml mcp
```

Use `MCP_ETCD_USERNAME` and `MCP_ETCD_PASSWORD` together for etcd authentication. They are never read from YAML or tool parameters. They must either both be set or both be absent.

`etcd_get` returns key metadata by default. Set `include_values: true` only when values are required. Supply each key or range endpoint as exactly one of a UTF-8 JSON string (`key`, `range_end`) or padded RFC 4648 Base64 (`key_base64`, `range_end_base64`); binary values are returned as `*_base64`. Every public 64-bit integer input and output is a canonical decimal JSON string, so MCP clients do not lose precision. `limit` defaults to `etcd_max_items` and cannot exceed it. A response larger than `etcd_max_response_bytes` fails without partial JSON.

## Transports and deployment boundary

Stdio is the default transport. HTTP mode retains `/healthz`, `/mcp`, `/sse`, and `/message`:

```bash
./bin/etcd-mcp-server --config config.example.yaml mcp --port 8080 --listen 127.0.0.1
```

For non-loopback HTTP/SSE deployments, the external network administrator owns inbound access control and incident response. This repository does not implement or validate inbound authentication, TLS termination, reverse proxies, or network segmentation.

## Optional integration test

The fixed unit suite does not contact etcd. A real v3.5.33 environment is opt-in:

```bash
MCP_ETCD_INTEGRATION_ENDPOINTS=https://127.0.0.1:2379 go test -tags=integration ./...
```

The only mandatory integration variable is `MCP_ETCD_INTEGRATION_ENDPOINTS`. Optional TLS inputs are `MCP_ETCD_INTEGRATION_CA_FILE` and the paired `MCP_ETCD_INTEGRATION_CERT_FILE` / `MCP_ETCD_INTEGRATION_KEY_FILE`; the runtime credential pair is reused when required. Optional fixtures are `MCP_ETCD_INTEGRATION_GET_KEY_BASE64`, `MCP_ETCD_INTEGRATION_LEASE_ID`, `MCP_ETCD_INTEGRATION_USER`, and `MCP_ETCD_INTEGRATION_ROLE`. Each missing fixture skips only its matching subtest. Missing integration endpoints cause a clear suite skip; they never mask unit-test failures.

## Development

```bash
go test ./...
go vet ./...
test -z "$(gofmt -l $(rg --files -g '*.go'))"
go mod verify
make build
```
