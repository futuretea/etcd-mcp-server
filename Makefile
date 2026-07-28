BINARY_NAME ?= etcd-mcp-server
GOLANGCI_LINT ?= golangci-lint
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT ?= $(shell git rev-parse --verify --quiet HEAD 2>/dev/null || echo unknown)
DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -X github.com/futuretea/etcd-mcp-server/pkg/core/version.Version=$(VERSION) \
	-X github.com/futuretea/etcd-mcp-server/pkg/core/version.Commit=$(COMMIT) \
	-X github.com/futuretea/etcd-mcp-server/pkg/core/version.Date=$(DATE)

NPM_PACKAGE ?= etcd-mcp-server
NPM_ROOT ?= npm
NPM_VERSION ?= $(shell git describe --tags --always 2>/dev/null | sed 's/^v//')
NPM_PUBLISH_FLAGS ?= --access=public
NPM_PACK_DIR ?= dist/npm-packages
NPM ?= npm
NODE ?= node
NPM_UNIX_PLATFORMS := darwin-amd64 darwin-arm64 linux-amd64 linux-arm64
NPM_WINDOWS_PLATFORMS := windows-amd64
ETCD_E2E_IMAGE ?= quay.io/coreos/etcd:v3.5.33

.PHONY: build test test-e2e lint format tidy ci clean docker coverage npm-test-launcher npm-verify-binaries npm-prepare-packages npm-create-tarballs npm-publish

build:
	go build -ldflags "$(LDFLAGS)" -o bin/$(BINARY_NAME) ./cmd/etcd-mcp-server

test:
	go test ./...

test-e2e:
	@set -eu; \
		container="$$(docker run --rm -d -P "$(ETCD_E2E_IMAGE)" /usr/local/bin/etcd --name e2e --data-dir /etcd-data --listen-client-urls http://0.0.0.0:2379 --advertise-client-urls http://0.0.0.0:2379)"; \
		cleanup() { docker stop "$$container" >/dev/null 2>&1 || true; }; \
		trap cleanup EXIT; \
		for attempt in $$(seq 1 30); do \
			if docker exec "$$container" etcdctl endpoint health >/dev/null 2>&1; then break; fi; \
			if [ "$$attempt" = 30 ]; then docker logs "$$container"; exit 1; fi; \
			sleep 1; \
		done; \
		docker exec "$$container" etcdctl put /e2e/get value; \
		lease_hex="$$(docker exec "$$container" etcdctl lease grant 60 | sed -n 's/^lease \([^ ]*\) granted.*/\1/p')"; \
		test -n "$$lease_hex"; \
		docker exec "$$container" etcdctl put --lease="$$lease_hex" /e2e/lease value; \
		lease_id="$$(printf '%d' "0x$$lease_hex")"; \
		port="$$(docker port "$$container" 2379/tcp | sed -n '1s/.*://p')"; \
		test -n "$$port"; \
		MCP_ETCD_INTEGRATION_ENDPOINTS="http://127.0.0.1:$$port" \
		MCP_ETCD_INTEGRATION_GET_KEY_BASE64="$$(printf '%s' /e2e/get | base64)" \
		MCP_ETCD_INTEGRATION_LEASE_ID="$$lease_id" \
		MCP_ETCD_INTEGRATION_CA_FILE= \
		MCP_ETCD_INTEGRATION_CERT_FILE= \
		MCP_ETCD_INTEGRATION_KEY_FILE= \
		MCP_ETCD_INTEGRATION_USER= \
		MCP_ETCD_INTEGRATION_ROLE= \
		MCP_ETCD_USERNAME= \
		MCP_ETCD_PASSWORD= \
		go test -v -tags=integration ./integration

lint:
	go vet ./...
	test -z "$$(gofmt -l $$(rg --files -g '*.go'))"
	@if command -v "$(GOLANGCI_LINT)" >/dev/null 2>&1; then \
		$(GOLANGCI_LINT) run ./...; \
	else \
		echo "$(GOLANGCI_LINT) not installed, skipping"; \
	fi

format:
	gofmt -w $$(find cmd internal pkg -name '*.go')

tidy:
	go mod tidy

coverage:
	go test ./pkg/... -coverprofile=coverage.out
	go tool cover -func=coverage.out | tail -1

ci: lint test test-e2e build

DOCKER_TAG ?= dev

docker:
	docker build \
		--build-arg VERSION=$(VERSION) \
		--build-arg GIT_COMMIT=$(COMMIT) \
		--build-arg BUILD_DATE=$(DATE) \
		-t etcd-mcp-server:$(DOCKER_TAG) \
		-t etcd-mcp-server:$(VERSION) .

npm-test-launcher:
	$(NODE) --test npm/$(NPM_PACKAGE)/test/launcher.test.js

npm-verify-binaries:
	@set -eu; \
		for platform in $(NPM_UNIX_PLATFORMS); do \
			test -x "$(NPM_ROOT)/$(NPM_PACKAGE)-$$platform/bin/$(BINARY_NAME)"; \
		done; \
		for platform in $(NPM_WINDOWS_PLATFORMS); do \
			test -f "$(NPM_ROOT)/$(NPM_PACKAGE)-$$platform/bin/$(BINARY_NAME).exe"; \
		done

npm-prepare-packages: npm-verify-binaries
	test -f README.md
	test -f LICENSE
	cp README.md LICENSE "$(NPM_ROOT)/$(NPM_PACKAGE)/"
	@set -eu; for package_dir in "$(NPM_ROOT)"/$(NPM_PACKAGE)-*; do \
		jq --arg version "$(NPM_VERSION)" '.version = $$version' "$$package_dir/package.json" > "$$package_dir/package.json.tmp"; \
		mv "$$package_dir/package.json.tmp" "$$package_dir/package.json"; \
	done
	jq --arg version "$(NPM_VERSION)" '.version = $$version | .optionalDependencies |= with_entries(.value = $$version)' "$(NPM_ROOT)/$(NPM_PACKAGE)/package.json" > "$(NPM_ROOT)/$(NPM_PACKAGE)/package.json.tmp"
	mv "$(NPM_ROOT)/$(NPM_PACKAGE)/package.json.tmp" "$(NPM_ROOT)/$(NPM_PACKAGE)/package.json"

npm-create-tarballs: npm-prepare-packages
	rm -rf "$(NPM_PACK_DIR)"
	mkdir -p "$(NPM_PACK_DIR)"
	@set -eu; for package_dir in "$(NPM_ROOT)"/$(NPM_PACKAGE)-* "$(NPM_ROOT)/$(NPM_PACKAGE)"; do \
		filename="$$(cd "$$package_dir" && $(NPM) pack --json | jq -er '.[0].filename')"; \
		mv "$$package_dir/$$filename" "$(NPM_PACK_DIR)/$$(basename "$$package_dir").tgz"; \
	done

npm-publish: npm-create-tarballs
	@set -eu; \
		temp_dir="$$(mktemp -d)"; \
		trap 'rm -rf "$$temp_dir"' EXIT; \
		for package_dir in "$(NPM_ROOT)"/$(NPM_PACKAGE)-* "$(NPM_ROOT)/$(NPM_PACKAGE)"; do \
			package_name="$$(jq -er '.name' "$$package_dir/package.json")"; \
			package_version="$$(jq -er '.version' "$$package_dir/package.json")"; \
			package_spec="$$package_name@$$package_version"; \
			archive="$(NPM_PACK_DIR)/$$(basename "$$package_dir").tgz"; \
			local_integrity="$$( $(NODE) -e 'const crypto = require("crypto"); const fs = require("fs"); process.stdout.write("sha512-" + crypto.createHash("sha512").update(fs.readFileSync(process.argv[1])).digest("base64"));' "$$archive")"; \
			if $(NPM) view "$$package_spec" dist.integrity --json > "$$temp_dir/view.json" 2> "$$temp_dir/view.err"; then \
				remote_integrity="$$(jq -er 'if type == "string" and length > 0 then . else error("missing dist.integrity") end' "$$temp_dir/view.json")"; \
				if [ "$$local_integrity" != "$$remote_integrity" ]; then \
					echo "Existing $$package_spec does not match the local release artifact" >&2; \
					exit 1; \
				fi; \
				echo "$$package_spec is already published with matching integrity"; \
			elif grep -Eq 'E404|404 Not Found' "$$temp_dir/view.err"; then \
				NPM_PACKAGE_NAME="$$package_name" NPM_PACKAGE_VERSION="$$package_version" $(NPM) publish "$$archive" $(NPM_PUBLISH_FLAGS); \
			else \
				cat "$$temp_dir/view.err" >&2; \
				exit 1; \
			fi; \
		done

clean:
	rm -rf bin coverage.out coverage.txt
