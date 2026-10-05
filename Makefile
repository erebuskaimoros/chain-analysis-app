SHELL := /bin/bash

.PHONY: build-server build-ui build-cli restart-server stop-server run-server

build-ui:
	@if [ ! -d frontend/node_modules ]; then npm --prefix frontend install --no-fund --no-audit; fi
	npm --prefix frontend run build

build-server: build-ui
	@COMMIT=$$(git rev-parse --short HEAD 2>/dev/null || echo unknown); \
	BUILD_TIME=$$(date -u +%Y-%m-%dT%H:%M:%SZ); \
	VERSION=$${CHAIN_ANALYSIS_BUILD_VERSION:-dev}; \
	LDFLAGS="-X main.version=$${VERSION} -X main.commit=$${COMMIT} -X main.buildTime=$${BUILD_TIME}"; \
	go build -ldflags "$${LDFLAGS}" -o data/bin/chain-analysis-server ./cmd/server

build-cli:
	go build -o data/bin/cactl ./cmd/cactl
	go build -o data/bin/cactl-mcp ./cmd/cactl-mcp

restart-server:
	./scripts/restart-server.sh restart

stop-server:
	./scripts/restart-server.sh stop

run-server:
	go run ./cmd/server
