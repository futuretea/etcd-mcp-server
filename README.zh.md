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

## 安装发布版本

推送以 `v` 开头的 tag 会分别触发 npm 和 Docker 发布工作流。npm 工作流发布启动器和五个原生包：macOS（amd64、arm64）、Linux（amd64、arm64）与 Windows（amd64），随后创建 GitHub Release。Docker 工作流向 GitHub Container Registry（GHCR）发布 `linux/amd64` 与 `linux/arm64` 镜像。

npm 包使用去掉前导 `v` 后的 tag 文本作为版本号，因此该文本必须是合法的 npm 版本。Docker 工作流把完整 tag 用作镜像 tag，维护者必须使用同时兼容 npm 与 Docker 的 tag。例如 npm 接受 `v1.2.3+build.7`，但 Docker 镜像 tag 不接受 `+`，该发布可能只发布 npm 制品。维护者必须先在仓库 Secrets 中配置 `NPM_TOKEN`，npm 工作流才能发布。

### npm

用 `npx` 运行已发布版本：

```bash
npx -y @futuretea/etcd-mcp-server@<version> --config ./config.yaml
```

MCP client 配置示例：

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

npm 命令始终启动 MCP 服务。请固定所需版本，保证部署可复现。

### Docker

使用只读配置挂载运行已发布镜像：

```bash
docker run --rm -i \
  --mount type=bind,src="$PWD/config.yaml",dst=/etc/etcd-mcp-server/config.yaml,readonly \
  ghcr.io/futuretea/etcd-mcp-server:<tag> \
  --config /etc/etcd-mcp-server/config.yaml
```

如果 MCP client 通过 Docker 启动服务，使用同一镜像并将配置挂载参数传递给 Docker 进程。当配置的 etcd 集群需要认证时，必须同时传入 `MCP_ETCD_USERNAME` 与 `MCP_ETCD_PASSWORD`。

当配置引用 TLS 文件时，也要挂载其所在目录，并在 `etcd_ca_file`、`etcd_cert_file` 和 `etcd_key_file` 中使用容器内路径：

```bash
--mount type=bind,src="$PWD/tls",dst=/etc/etcd-mcp-server/tls,readonly
```

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
