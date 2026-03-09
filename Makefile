SHELL := /bin/bash

.PHONY: build-server restart-server stop-server run-server

build-server:
	@COMMIT=$$(git rev-parse --short HEAD 2>/dev/null || echo unknown); \
	BUILD_TIME=$$(date -u +%Y-%m-%dT%H:%M:%SZ); \
	VERSION=$${CHAIN_ANALYSIS_BUILD_VERSION:-dev}; \
	LDFLAGS="-X main.version=$${VERSION} -X main.commit=$${COMMIT} -X main.buildTime=$${BUILD_TIME}"; \
	go build -ldflags "$${LDFLAGS}" -o data/bin/chain-analysis-server ./cmd/server

restart-server:
	./scripts/restart-server.sh restart

stop-server:
	./scripts/restart-server.sh stop

run-server:
	go run ./cmd/server
