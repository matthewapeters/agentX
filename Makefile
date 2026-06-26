# AgentX repository Makefile
#
# Canonical contract (docs/implementation/09_makefile_and_quality_gate_contract.md):
#   make all = make clean && make build
#   - if clean fails, build must not run
#   - any failure exits non-zero
#
# Single runtime command is anchored at cmd/agentx
# (docs/implementation/08_go_module_layout.md).

GO       ?= go
CMD_DIR  := ./cmd/agentx
BIN_DIR  := bin
BIN      := $(BIN_DIR)/agentx

.PHONY: help all build clean test go-test \
	go-test-unit go-test-integration go-test-functional go-test-e2e \
	vendor-check run

help:
	@echo "AgentX Make Targets"
	@echo ""
	@echo "Baseline:"
	@echo "  all                 Canonical gate: clean then build (required before merge)"
	@echo ""
	@echo "Build:"
	@echo "  build               Compile the agentx runtime into $(BIN)"
	@echo "  clean               Remove build artifacts"
	@echo ""
	@echo "Test:"
	@echo "  test                Run all Go + Godog tests"
	@echo "  go-test             Alias for test"
	@echo "  go-test-unit        Run Godog @unit suite"
	@echo "  go-test-integration Run Godog @integration suite"
	@echo "  go-test-functional  Run Godog @functional suite"
	@echo "  go-test-e2e         Run Godog @e2e suite"
	@echo ""
	@echo "Hygiene:"
	@echo "  vendor-check        Verify go.mod/vendor are consistent"
	@echo ""
	@echo "Run:"
	@echo "  run                 Build and run the agentx runtime"

all:
	@$(MAKE) clean && $(MAKE) build
	@echo "Baseline verification complete"

build:
	@echo "Validating (vet + tests)..."
	$(GO) vet ./...
	$(GO) test ./...
	@echo "Building agentx..."
	@mkdir -p $(BIN_DIR)
	$(GO) build -o $(BIN) $(CMD_DIR)
	@echo "agentx built at $(BIN)"

clean:
	@echo "Cleaning artifacts..."
	@rm -rf $(BIN_DIR)
	@$(GO) clean
	@echo "Clean complete"

test: go-test

go-test:
	$(GO) test ./...

# Godog suites are tag-scoped (docs/implementation/07_test_and_documentation_contract.md).
# Suite runners live under tests/suites and select scenarios by tag.
go-test-unit:
	$(GO) test -run 'TestUnit' ./tests/...

go-test-integration:
	$(GO) test -run 'TestIntegration' ./tests/...

go-test-functional:
	$(GO) test -run 'TestFunctional' ./tests/...

go-test-e2e:
	$(GO) test -run 'TestE2E' ./tests/...

vendor-check:
	$(GO) mod verify
	$(GO) build -mod=vendor ./...

run: build
	./$(BIN)
