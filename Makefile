# AgentX repository Makefile

GO_CORE_DIR := cmd/agentx-core
GO_CORE_BIN := bin/agentx

.PHONY: help \
	build build-core build-applets clean \
	test go-test go-test-unit go-test-integration go-test-functional go-test-e2e \
	run run-with-applets

help:
	@echo "AgentX Make Targets"
	@echo ""
	@echo "Build:"
	@echo "  build               Build Go core and prepare applets"
	@echo "  build-core          Build Go core binary only"
	@echo "  build-applets       Copy Python applets into bin/applets"
	@echo "  clean               Remove build artifacts"
	@echo ""
	@echo "Test:"
	@echo "  test                Alias for go-test"
	@echo "  go-test             Run all Go tests in cmd/agentx-core"
	@echo "  go-test-unit        Run GoDog @unit suite"
	@echo "  go-test-integration Run GoDog @integration suite"
	@echo "  go-test-functional  Run GoDog @functional suite"
	@echo "  go-test-e2e         Run GoDog @e2e suite"
	@echo ""
	@echo "Run:"
	@echo "  run                 Build and run Go core"
	@echo "  run-with-applets    Build and run Go core with applets prepared"

build: build-core build-applets
	@echo "Build complete"

build-core:
	@echo "Building Go core..."
	@mkdir -p bin
	cd $(GO_CORE_DIR) && go mod tidy
	cd $(GO_CORE_DIR) && go mod download
	cd $(GO_CORE_DIR) && go build -o ../../$(GO_CORE_BIN)
	@chmod +x $(GO_CORE_BIN)
	@echo "Go core built at $(GO_CORE_BIN)"

build-applets:
	@echo "Preparing Python applets..."
	@mkdir -p bin/applets
	@cp -v applets/*.py bin/applets/ 2>/dev/null || true
	@echo "Applets prepared"

clean:
	@echo "Cleaning artifacts..."
	@rm -rf bin/
	@cd $(GO_CORE_DIR) && go clean
	@echo "Clean complete"

test: go-test

go-test:
	cd $(GO_CORE_DIR) && go test -v ./...

go-test-unit:
	cd $(GO_CORE_DIR) && go test -v -run TestGoDogUnit ./...

go-test-integration:
	cd $(GO_CORE_DIR) && go test -v -run TestGoDogIntegration ./...

go-test-functional:
	cd $(GO_CORE_DIR) && go test -v -run TestGoDogFunctional ./...

go-test-e2e:
	cd $(GO_CORE_DIR) && go test -v -run TestGoDogE2E ./...

run: build-core
	@echo "Running Go core..."
	./$(GO_CORE_BIN) --project-dir . --user $$USER

run-with-applets: build
	@echo "Running Go core with applets prepared..."
	./$(GO_CORE_BIN) --project-dir . --user $$USER