# etcd MCP Server

[中文文档](README.zh.md)

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

## Install a release

Pushing a tag that starts with `v` runs independent npm and Docker release workflows. The npm workflow publishes the launcher and native packages for macOS (amd64 and arm64), Linux (amd64 and arm64), and Windows (amd64), then creates the GitHub release. The Docker workflow publishes `linux/amd64` and `linux/arm64` images to GitHub Container Registry (GHCR).

The npm package uses the tag text after the leading `v` as its package version, so it must be a valid npm version. The Docker workflow uses the complete tag as its image tag; maintainers must use a tag compatible with both npm and Docker. For example, npm accepts `v1.2.3+build.7`, but Docker does not accept `+` in an image tag and that release can publish only npm artifacts. Maintainers must set the repository `NPM_TOKEN` secret before the npm workflow can publish.

### npm

Run a released version with `npx`:

```bash
npx -y @futuretea/etcd-mcp-server@<version> --config ./config.yaml
```

For an MCP client configuration:

```json
{
  "mcpServers": {
    "etcd-readonly": {
      "command": "npx",
      "args": ["-y", "@futuretea/etcd-mcp-server@<version>", "--config", "/absolute/path/to/config.yaml"]
    }
  }
}
```

The npm command always starts the MCP server. Use a pinned package version for reproducible deployments.

### Docker

Run a released image with a read-only config mount:

```bash
docker run --rm -i \
  --mount type=bind,src="$PWD/config.yaml",dst=/etc/etcd-mcp-server/config.yaml,readonly \
  ghcr.io/futuretea/etcd-mcp-server:<tag> \
  --config /etc/etcd-mcp-server/config.yaml
```

For Docker-based MCP clients, use the same image and pass the config mount to the Docker process. Add `MCP_ETCD_USERNAME` and `MCP_ETCD_PASSWORD` together when the configured etcd cluster requires authentication.

When the config names TLS files, mount their containing directory too and use the container paths in `etcd_ca_file`, `etcd_cert_file`, and `etcd_key_file`:

```bash
--mount type=bind,src="$PWD/tls",dst=/etc/etcd-mcp-server/tls,readonly
```

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
