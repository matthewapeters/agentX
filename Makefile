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

# Logo asset: authored source (ANSI-colored text) vs. its build-time
# conversion into Go source. cmd/logogen parses the ANSI escapes into a
# structured (rune, xterm-256 color index) cell grid and generates LOGO_DST,
# so internal/surfaces/banner never parses escape sequences at runtime and its
# color-cycle animation is agnostic to the raw ANSI content — see
# logo/README.md for the full pipeline and rationale. The banner's collapsed
# row ("AgentX - <activity>", docs/ux/06_OUTPUT_WIDGET.md "Logo banner") is
# NOT a generated asset — its text varies with what the agent is doing, so
# internal/surfaces/banner synthesizes it at runtime instead.
#
# LOGOGEN_BIN must be built before LOGO_DST can be generated, and LOGO_DST
# must exist before the rest of the application can be compiled (it defines
# LogoGrid, which internal/surfaces/banner references) — the codegen tool is
# therefore always built ahead of the application binary; see the rules below
# and their placement before the `build` recipe runs.
LOGO_SRC    := logo/agentx.logo
LOGO_DST    := internal/surfaces/banner/logo_generated.go
LOGOGEN_SRC := ./cmd/logogen
LOGOGEN_BIN := $(BIN_DIR)/logogen

# Default-config seed: baseline files installed into the user's config dir. The
# packaging step named as "future work" in config/seed/README.md — copies each
# baseline into place without clobbering an existing (user-tuned) copy.
SEED_DIR    := config/seed
CONFIG_HOME ?= $(or $(XDG_CONFIG_HOME),$(HOME)/.config)
CONFIG_DIR  := $(CONFIG_HOME)/agentx

# Install layout (GNU convention): a no-sudo user install by default, landing in
# a dir already on most PATHs. Override for a system install:
#   make install PREFIX=/usr/local        (needs sudo)
# DESTDIR stages the tree elsewhere for packaging.
PREFIX  ?= $(HOME)/.local
BINDIR  ?= $(PREFIX)/bin
DESTDIR ?=

# Version stamped into the binary via -ldflags (main.version). Derived from git
# so an installed build is identifiable; falls back to "dev" outside a checkout.
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -X main.version=$(VERSION)

.PHONY: help all build clean test go-test \
	go-test-unit go-test-integration go-test-functional go-test-e2e \
	vendor-check run seed install install-bin uninstall doctor install-deps

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
	@echo "Install:"
	@echo "  install             Build + seed + install agentx & ax into $(BINDIR), then doctor"
	@echo "  install-bin         Copy the built binary + ax into $(BINDIR)"
	@echo "  uninstall           Remove installed binaries (config left intact)"
	@echo "  doctor              Report binary/config/dep health (non-fatal)"
	@echo "  install-deps        Best-effort install of zellij (opt-in)"
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
	@echo "Building agentx ($(VERSION))..."
	@mkdir -p $(BIN_DIR)
	$(GO) build -ldflags "$(LDFLAGS)" -o $(BIN) $(CMD_DIR)
	@echo "agentx built at $(BIN)"

# Build the logo codegen tool. A plain file-target (not .PHONY) so Make only
# rebuilds it when its sources change, same as any other compiled artifact.
$(LOGOGEN_BIN): $(wildcard $(LOGOGEN_SRC)/*.go)
	@mkdir -p $(dir $@)
	$(GO) build -o $@ $(LOGOGEN_SRC)

# Regenerate the logo's Go source when its authored ANSI source changes (or
# the generator itself changed). Rendered to a .tmp file first and swapped in
# only on a content diff, so an unchanged banner doesn't dirty the tree or
# bust downstream build caching.
$(LOGO_DST): $(LOGO_SRC) $(LOGOGEN_BIN)
	@mkdir -p $(dir $@)
	@$(LOGOGEN_BIN) -in $(LOGO_SRC) -out $@.tmp -pkg banner -var LogoGrid
	@if ! cmp -s $@.tmp $@; then echo "Logo changed; regenerating $@"; mv $@.tmp $@; else rm -f $@.tmp; touch $@; fi

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
	@echo "Seeding baseline config into $(CONFIG_DIR) (overwriting — customization retention not yet implemented)..."
	@mkdir -p "$(CONFIG_DIR)"
	@for f in $(SEED_DIR)/*; do \
		b=$$(basename "$$f"); \
		[ "$$b" = "README.md" ] && continue; \
		cp "$$f" "$(CONFIG_DIR)/$$b" && echo "  seed  $$b"; \
	done
	@echo "Seed complete"

# End-to-end install: build the (version-stamped) binary, seed config, drop the
# binary + the ax launcher onto PATH, then report on external deps. seed runs
# every install and overwrites the deployed baseline files with the shipped ones,
# so an upgrade always picks up prompt/config changes. Customization retention is
# not yet implemented, so local edits under $(CONFIG_DIR) are replaced on install.
install: build seed install-bin doctor
	@echo ""
	@echo "AgentX $(VERSION) installed to $(BINDIR). Launch with 'ax' (or 'agentx')."

# Copy the built binary and the ax launcher into BINDIR. Depends on build so the
# artifact carries the version stamp and passed the vet+test gate.
install-bin: build
	@echo "Installing binaries into $(DESTDIR)$(BINDIR)..."
	@mkdir -p "$(DESTDIR)$(BINDIR)"
	install -m 0755 $(BIN) "$(DESTDIR)$(BINDIR)/agentx"
	install -m 0755 ax     "$(DESTDIR)$(BINDIR)/ax"
	@echo "  installed  agentx"
	@echo "  installed  ax"

# Remove the installed binaries. Config in CONFIG_DIR is left intact — it may hold
# user-tuned files; remove it by hand if a clean slate is wanted.
uninstall:
	@rm -f "$(DESTDIR)$(BINDIR)/agentx" "$(DESTDIR)$(BINDIR)/ax"
	@echo "Removed agentx + ax from $(DESTDIR)$(BINDIR)."
	@echo "Config in $(CONFIG_DIR) left intact (remove manually for a clean slate)."

# Non-fatal health check: reports each requirement ✓/✗ with a one-line fix. Run
# standalone anytime, or as the last step of install. Never fails the build — a
# missing external dep should not block installing the binary.
doctor:
	@echo "AgentX doctor:"
	@case ":$$PATH:" in *":$(BINDIR):"*) echo "  \342\234\223 $(BINDIR) on PATH";; \
		*) echo "  \342\234\227 $(BINDIR) not on PATH — add: export PATH=\"$(BINDIR):\$$PATH\"";; esac
	@if command -v agentx >/dev/null 2>&1; then echo "  \342\234\223 agentx: $$(agentx --version 2>/dev/null)"; \
		else echo "  \342\234\227 agentx not on PATH"; fi
	@if command -v zellij >/dev/null 2>&1; then echo "  \342\234\223 zellij: $$(zellij --version 2>/dev/null)"; \
		else echo "  \342\234\227 zellij not found — 'ax' needs it (baseline 0.44.3). Try 'make install-deps'"; fi
	@if command -v ollama >/dev/null 2>&1; then echo "  \342\234\223 ollama present"; \
		else echo "  \342\234\227 ollama not found — required at runtime (https://ollama.com)"; fi
	@MODEL=$$(sed -n 's/^[[:space:]]*model[[:space:]]*=[[:space:]]*"\(.*\)".*/\1/p' "$(CONFIG_DIR)/agentx.toml" 2>/dev/null | head -1); \
		if [ -z "$$MODEL" ]; then echo "  - model: none configured in $(CONFIG_DIR)/agentx.toml"; \
		elif command -v ollama >/dev/null 2>&1 && ollama list 2>/dev/null | grep -q "$$MODEL"; then echo "  \342\234\223 model $$MODEL pulled"; \
		else echo "  \342\234\227 model $$MODEL not pulled — run: ollama pull $$MODEL"; fi
	@if [ -e "$(CONFIG_DIR)/prompts.toml" ]; then echo "  \342\234\223 prompts.toml installed (task detection armed)"; \
		else echo "  \342\234\227 prompts.toml missing — run 'make seed' (task detection stays OFF without it)"; fi

# Best-effort, opt-in install of the external harness dependency (zellij). Cross-
# distro package managers vary; this tries the common ones and otherwise points at
# the upstream docs rather than guessing.
install-deps:
	@if command -v zellij >/dev/null 2>&1; then echo "zellij already installed ($$(zellij --version))"; \
	elif command -v cargo >/dev/null 2>&1; then echo "Installing zellij via cargo..."; cargo install zellij; \
	elif command -v brew >/dev/null 2>&1; then echo "Installing zellij via brew..."; brew install zellij; \
	elif command -v pacman >/dev/null 2>&1; then echo "Installing zellij via pacman..."; sudo pacman -S --needed zellij; \
	else echo "No supported package manager found. Install zellij manually:"; \
		echo "  https://zellij.dev/documentation/installation"; fi
