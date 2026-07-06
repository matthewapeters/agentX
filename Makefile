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

# Logo asset: authored source vs. the embedded copy under the command tree. The
# build refreshes the copy whenever the source is newer so an edited logo is
# re-embedded automatically (docs/implementation/09_makefile_and_quality_gate_contract.md).
LOGO_SRC := logo/agentx.logo
LOGO_DST := cmd/agentx/assets/agentx.logo

# Default-config seed: baseline files installed into the user's config dir. The
# packaging step named as "future work" in config/seed/README.md — copies each
# baseline into place without clobbering an existing (user-tuned) copy.
SEED_DIR    := config/seed
CONFIG_HOME ?= $(or $(XDG_CONFIG_HOME),$(HOME)/.config)
CONFIG_DIR  := $(CONFIG_HOME)/agentx

.PHONY: help all build clean test go-test \
	go-test-unit go-test-integration go-test-functional go-test-e2e \
	vendor-check run seed

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
	@echo "Config:"
	@echo "  seed                Install baseline config into $(CONFIG_DIR) (keeps existing files)"
	@echo ""
	@echo "Run:"
	@echo "  run                 Build and run the agentx runtime"

all:
	@$(MAKE) clean && $(MAKE) build
	@echo "Baseline verification complete"

build: $(LOGO_DST)
	@echo "Validating (vet + tests)..."
	$(GO) vet ./...
	$(GO) test ./...
	@echo "Building agentx..."
	@mkdir -p $(BIN_DIR)
	$(GO) build -o $(BIN) $(CMD_DIR)
	@echo "agentx built at $(BIN)"

# Refresh the embedded logo copy when the authored source changes. Make's
# timestamp comparison drives "is it changed"; cmp avoids a no-op rewrite.
$(LOGO_DST): $(LOGO_SRC)
	@mkdir -p $(dir $@)
	@if ! cmp -s $< $@; then echo "Logo changed; updating $@"; cp $< $@; else touch $@; fi

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
#
# GOTEST_TIMEOUT bounds each suite: a scenario that spins is killed with a full
# goroutine dump instead of hanging to the 10m default or dying to a silent
# SIGKILL, so the stuck step is named. Belt-and-suspenders with the progress
# formatter (pretty-buffering was the OOM amplifier; see tests/suites).
GOTEST_TIMEOUT ?= 120s

go-test-unit:
	$(GO) test -timeout $(GOTEST_TIMEOUT) -run 'TestUnit' ./tests/...

go-test-integration:
	$(GO) test -timeout $(GOTEST_TIMEOUT) -run 'TestIntegration' ./tests/...

go-test-functional:
	$(GO) test -timeout $(GOTEST_TIMEOUT) -run 'TestFunctional' ./tests/...

go-test-e2e:
	$(GO) test -timeout $(GOTEST_TIMEOUT) -run 'TestE2E' ./tests/...

vendor-check:
	$(GO) mod verify
	$(GO) build -mod=vendor ./...

run: build
	./$(BIN)

# Install the baseline config files into the user's config dir, preserving any
# file already there (a user's tuned copy always wins). README.md is repo-facing
# documentation, not a deployed file, so it is skipped. The zellij harness layout
# (agentx.kdl) rides along here even though the agentx runtime never reads it.
seed:
	@echo "Seeding baseline config into $(CONFIG_DIR) (existing files preserved)..."
	@mkdir -p "$(CONFIG_DIR)"
	@for f in $(SEED_DIR)/*; do \
		b=$$(basename "$$f"); \
		[ "$$b" = "README.md" ] && continue; \
		if [ -e "$(CONFIG_DIR)/$$b" ]; then echo "  keep  $$b"; \
		else cp "$$f" "$(CONFIG_DIR)/$$b" && echo "  seed  $$b"; fi; \
	done
	@echo "Seed complete"
