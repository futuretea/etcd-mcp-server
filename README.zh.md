# etcd MCP Server

`etcd-mcp-server` 为 etcd v3.5.33 提供立即返回、只读的 MCP 查询工具。它保留 stdio、Streamable HTTP 和 SSE 传输，并只返回有边界的 JSON 文本。

## 工具范围

公开工具固定为：`etcd_get`、`etcd_lease_ttl`、`etcd_lease_list`、`etcd_member_list`、`etcd_alarm_list`、`etcd_status`、`etcd_hash_kv`、`etcd_auth_status`、`etcd_user_get`、`etcd_user_list`、`etcd_role_get`、`etcd_role_list`。

不提供写入、删除、事务、Auth 管理、Watch、Snapshot、RangeStream、reader 或 channel 工具。`etcd_status` 与 `etcd_hash_kv` 的 endpoint 必须精确匹配启动配置的 `etcd_endpoints`，MCP 参数不能成为新的拨号目标。

## 配置与启动

复制 [config.example.yaml](config.example.yaml)，填写 etcd endpoint 和可选 TLS 文件路径。默认拨号超时为 5s，每个工具请求超时为 10s，KV 最多 100 项，JSON 文本最多 1 MiB。

```bash
make build
./bin/etcd-mcp-server tools list --json
./bin/etcd-mcp-server --config config.example.yaml tools call etcd_get --params '{"key":"/applications/","prefix":true}'
./bin/etcd-mcp-server --config config.example.yaml mcp
```

认证仅从 `MCP_ETCD_USERNAME` 与 `MCP_ETCD_PASSWORD` 成对读取；它们不写入 YAML，也不能由 tool 参数传入。两个变量必须同时存在或同时缺省。

`etcd_get` 默认只返回 key 元数据。只有设置 `include_values: true` 才返回 value。每个 key 或 range endpoint 必须且只能使用 UTF-8 JSON 字符串（`key`、`range_end`）或带 padding 的 RFC 4648 Base64（`key_base64`、`range_end_base64`）之一传入；二进制输出使用 `*_base64`。所有公开的 64 位整数输入和输出都使用规范十进制 JSON 字符串，避免 MCP client 精度丢失。`limit` 默认等于 `etcd_max_items` 且不能超过它；超过 `etcd_max_response_bytes` 的结果明确失败，不返回部分 JSON。

## 传输与部署边界

stdio 是默认传输。HTTP 模式保留 `/healthz`、`/mcp`、`/sse`、`/message`：

```bash
./bin/etcd-mcp-server --config config.example.yaml mcp --port 8080 --listen 127.0.0.1
```

非 loopback HTTP/SSE 部署的入站网络控制和失控处置由外部网络管理员负责。本仓库不实现或验证入站认证、TLS 终止、反向代理或网络分段。

## 可选集成测试

固定单元测试不会连接 etcd。明确提供 v3.5.33 环境后可运行：

```bash
MCP_ETCD_INTEGRATION_ENDPOINTS=https://127.0.0.1:2379 go test -tags=integration ./...
```

唯一必填的 integration 变量是 `MCP_ETCD_INTEGRATION_ENDPOINTS`。可选 TLS 变量为 `MCP_ETCD_INTEGRATION_CA_FILE` 和成对的 `MCP_ETCD_INTEGRATION_CERT_FILE` / `MCP_ETCD_INTEGRATION_KEY_FILE`；需要认证时复用运行时凭据变量。可选 fixture 为 `MCP_ETCD_INTEGRATION_GET_KEY_BASE64`、`MCP_ETCD_INTEGRATION_LEASE_ID`、`MCP_ETCD_INTEGRATION_USER` 和 `MCP_ETCD_INTEGRATION_ROLE`，缺失时只 skip 对应子测试。缺少 integration endpoint 时整个套件清晰 skip，不能掩盖固定单元测试失败。

## 开发验证

```bash
go test ./...
go vet ./...
test -z "$(gofmt -l $(rg --files -g '*.go'))"
go mod verify
make build
```
