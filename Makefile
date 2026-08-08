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

# llama.cpp router service (optional; only needed when [agentx].provider =
# "llamacpp" in agentx.toml — see internal/config, `make doctor`). AgentX
# itself only ever talks to it over HTTP (internal/llm/llamacpp); these
# targets are a convenience for building/running that server, not a
# dependency of `make build`/`make all`.
#
# LLAMACPP_SRC_DIR is where `make llamacpp-clone`/`llamacpp-build` clone and
# build llama.cpp — point it at an existing checkout (e.g.
# LLAMACPP_SRC_DIR=/path/to/llama.cpp) to build that instead of cloning a
# fresh one. LLAMACPP_BACKEND auto-detects cuda vs cpu from nvcc on PATH;
# override explicitly with LLAMACPP_BACKEND=cpu to force a CPU-only build
# regardless of what's installed.
LLAMACPP_REPO      ?= https://github.com/ggml-org/llama.cpp.git
LLAMACPP_SRC_DIR    ?= $(HOME)/.local/share/agentx/llama.cpp
LLAMACPP_BACKEND    ?= $(shell command -v nvcc >/dev/null 2>&1 && echo cuda || echo cpu)
LLAMACPP_BUILD_DIR  := $(LLAMACPP_SRC_DIR)/build-$(LLAMACPP_BACKEND)
LLAMACPP_JOBS       ?= $(shell nproc 2>/dev/null || echo 4)

# Where `make llamacpp-install` installs the built binaries + shared
# libraries via `cmake --install` (llama.cpp's CMakeLists.txt has proper
# install() rules under GNUInstallDirs — bin/ and lib/ end up as separate
# directories here, unlike the raw build tree where CMake colocates them for
# in-place execution). /opt is root-owned on a standard install, so this
# needs sudo, same as the service targets below. This is the value
# LLAMACPP_PREFIX in /etc/default/llama-server should point at.
LLAMACPP_INSTALL_PREFIX ?= /opt/llama.cpp

# Default GGUF models location — deliberately NOT under a user's home
# directory: a home dir's own permissions (e.g. 750) commonly block
# traversal for a dedicated system service account regardless of how open
# the models dir itself is, whereas /opt is world-traversable by default, so
# nothing extra needs to be granted to the llama-server service user. This
# is the value LLAMA_ARG_MODELS_DIR in /etc/default/llama-server should
# point at.
LLAMACPP_MODELS_DIR ?= /opt/llama-server/models

# Install locations for the systemd unit (system-level, like this repo's
# ollama.service reference — needs sudo). LLAMACPP_SERVICE_USER is a
# dedicated, no-login system account the unit runs as (mirrors ollama's own
# service-user pattern), created idempotently by llamacpp-service-user.
LLAMACPP_SERVICE_USER   ?= llama-server
LLAMACPP_SERVICE_LIBDIR := /usr/local/lib/agentx
LLAMACPP_ENV_FILE       := /etc/default/llama-server
LLAMACPP_UNIT_FILE      := /etc/systemd/system/llama-server.service

.PHONY: help all build clean test go-test \
	go-test-unit go-test-integration go-test-functional go-test-e2e \
	vendor-check run seed install install-bin uninstall doctor install-deps \
	llamacpp-clone llamacpp-build llamacpp-install llamacpp-models-dir \
	llamacpp-service-user llamacpp-service-install llamacpp-service-uninstall

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
	@echo ""
	@echo "llama.cpp (optional; for provider = \"llamacpp\" in agentx.toml):"
	@echo "  llamacpp-clone            Clone llama.cpp into LLAMACPP_SRC_DIR (skips if already present)"
	@echo "  llamacpp-build            cmake configure+build (LLAMACPP_BACKEND=cuda|cpu, auto-detected)"
	@echo "  llamacpp-install          cmake --install into LLAMACPP_INSTALL_PREFIX (sudo, default /opt/llama.cpp)"
	@echo "  llamacpp-models-dir       Create LLAMACPP_MODELS_DIR (sudo, default /opt/llama-server/models)"
	@echo "  llamacpp-service-user     Create the dedicated llama-server system user (sudo, idempotent)"
	@echo "  llamacpp-service-install  Deploy the whole service end to end: build+install the binary, service user, models dir, unit+env file (sudo)"
	@echo "  llamacpp-service-uninstall Remove the systemd unit + wrapper script (sudo)"

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
	@LLAMACPP_HOST=$$(awk '/^\[agentx\.llamacpp\]/{f=1;next} /^\[/{f=0} f && /^[[:space:]]*host[[:space:]]*=/{gsub(/^[[:space:]]*host[[:space:]]*=[[:space:]]*"/,""); gsub(/".*/,""); print; exit}' "$(CONFIG_DIR)/agentx.toml" 2>/dev/null); \
		LLAMACPP_HOST=$${LLAMACPP_HOST:-localhost:8888}; \
		if command -v curl >/dev/null 2>&1 && curl -sf "http://$$LLAMACPP_HOST/v1/models" >/dev/null 2>&1; then echo "  \342\234\223 llama.cpp server reachable at $$LLAMACPP_HOST"; \
		else echo "  \342\234\227 llama.cpp server not reachable at $$LLAMACPP_HOST — start llama-server (https://github.com/ggml-org/llama.cpp), or ignore if using provider = \"ollama\""; fi
	@LLAMACPP_HOST=$$(awk '/^\[agentx\.llamacpp\]/{f=1;next} /^\[/{f=0} f && /^[[:space:]]*host[[:space:]]*=/{gsub(/^[[:space:]]*host[[:space:]]*=[[:space:]]*"/,""); gsub(/".*/,""); print; exit}' "$(CONFIG_DIR)/agentx.toml" 2>/dev/null); \
		LLAMACPP_HOST=$${LLAMACPP_HOST:-localhost:8888}; \
		LLAMACPP_MODEL=$$(awk '/^\[agentx\.llamacpp\]/{f=1;next} /^\[/{f=0} f && /^[[:space:]]*model[[:space:]]*=/{gsub(/^[[:space:]]*model[[:space:]]*=[[:space:]]*"/,""); gsub(/".*/,""); print; exit}' "$(CONFIG_DIR)/agentx.toml" 2>/dev/null); \
		if [ -z "$$LLAMACPP_MODEL" ]; then echo "  - llama.cpp model: none configured in $(CONFIG_DIR)/agentx.toml"; \
		elif command -v curl >/dev/null 2>&1 && curl -sf "http://$$LLAMACPP_HOST/v1/models" 2>/dev/null | grep -q "$$LLAMACPP_MODEL"; then echo "  \342\234\223 llama.cpp model $$LLAMACPP_MODEL loaded"; \
		else echo "  \342\234\227 llama.cpp model $$LLAMACPP_MODEL not confirmed loaded on $$LLAMACPP_HOST"; fi
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

# --- llama.cpp router service ------------------------------------------------
#
# Optional: only relevant when running AgentX against a local llama.cpp
# server instead of Ollama (agentx.toml's [agentx].provider = "llamacpp").
# AgentX itself only talks to this over HTTP (internal/llm/llamacpp) — these
# targets just build/run that server; nothing here is a dependency of `make
# build`/`make all`.

# Clones llama.cpp into LLAMACPP_SRC_DIR. A no-op if that path already looks
# like a checkout (has .git) — this never auto-pulls, since silently moving
# an existing checkout to a new upstream commit could invalidate a build the
# service currently depends on; update it yourself when you want to.
llamacpp-clone:
	@if [ -d "$(LLAMACPP_SRC_DIR)/.git" ]; then \
		echo "llama.cpp already present at $(LLAMACPP_SRC_DIR) (git -C $(LLAMACPP_SRC_DIR) pull to update)"; \
	else \
		echo "Cloning $(LLAMACPP_REPO) into $(LLAMACPP_SRC_DIR)..."; \
		mkdir -p "$$(dirname $(LLAMACPP_SRC_DIR))"; \
		git clone --depth 1 "$(LLAMACPP_REPO)" "$(LLAMACPP_SRC_DIR)"; \
	fi

# cmake configure+build. LLAMACPP_BACKEND is auto-detected (cuda if nvcc is
# on PATH, else cpu) but always overridable: `make llamacpp-build
# LLAMACPP_BACKEND=cpu`. Output lands in $(LLAMACPP_BUILD_DIR) — a raw build
# tree (CMake colocates binaries + shared libs there for in-place execution,
# with a build-tree RPATH baked in); run `make llamacpp-install` next to get
# a real, stable install location instead of running out of this directory.
llamacpp-build: llamacpp-clone
	@echo "Building llama.cpp ($(LLAMACPP_BACKEND) backend) into $(LLAMACPP_BUILD_DIR)..."
	@if [ "$(LLAMACPP_BACKEND)" = "cuda" ]; then \
		cmake -S "$(LLAMACPP_SRC_DIR)" -B "$(LLAMACPP_BUILD_DIR)" -DCMAKE_BUILD_TYPE=Release -DGGML_CUDA=ON; \
	else \
		cmake -S "$(LLAMACPP_SRC_DIR)" -B "$(LLAMACPP_BUILD_DIR)" -DCMAKE_BUILD_TYPE=Release; \
	fi
	cmake --build "$(LLAMACPP_BUILD_DIR)" --config Release -j$(LLAMACPP_JOBS)
	@echo "llama.cpp built at $(LLAMACPP_BUILD_DIR)"
	@echo "Run 'sudo make llamacpp-install' to install it to $(LLAMACPP_INSTALL_PREFIX)."

# `cmake --install` into LLAMACPP_INSTALL_PREFIX (default /opt/llama.cpp,
# root-owned — needs sudo). This is the step the raw build tree skips:
# llama.cpp's CMakeLists.txt has real install() rules (GNUInstallDirs), which
# split binaries into <prefix>/bin and shared libraries into <prefix>/lib and
# strip the build-tree RPATH. llamacpp-service-install seeds
# $(LLAMACPP_ENV_FILE) with LLAMACPP_PREFIX=$(LLAMACPP_INSTALL_PREFIX)
# automatically (substituted from this same variable, not a hardcoded
# guess) — if you override LLAMACPP_INSTALL_PREFIX here, use the same
# override there, or edit $(LLAMACPP_ENV_FILE) by hand afterward.
llamacpp-install: llamacpp-build
	@if [ "$$(id -u)" != "0" ]; then echo "requires root — run: sudo make llamacpp-install" >&2; exit 1; fi
	cmake --install "$(LLAMACPP_BUILD_DIR)" --prefix "$(LLAMACPP_INSTALL_PREFIX)"
	@echo "llama.cpp installed at $(LLAMACPP_INSTALL_PREFIX) (bin/, lib/)."
	@if [ -e "$(LLAMACPP_ENV_FILE)" ] && ! grep -q "^LLAMACPP_PREFIX=$(LLAMACPP_INSTALL_PREFIX)$$" "$(LLAMACPP_ENV_FILE)" 2>/dev/null; then \
		echo "NOTE: $(LLAMACPP_ENV_FILE) already exists and does not have LLAMACPP_PREFIX=$(LLAMACPP_INSTALL_PREFIX) — update it by hand."; \
	fi

# Creates the dedicated, no-login system account llama-server.service runs
# as — mirrors this host's existing ollama.service pattern (system user,
# private group, video+render group membership for GPU device access).
# Idempotent; requires root.
llamacpp-service-user:
	@if [ "$$(id -u)" != "0" ]; then echo "requires root — run: sudo make llamacpp-service-user" >&2; exit 1; fi
	@if getent passwd $(LLAMACPP_SERVICE_USER) >/dev/null 2>&1; then \
		echo "service user $(LLAMACPP_SERVICE_USER) already exists"; \
	else \
		echo "Creating system user $(LLAMACPP_SERVICE_USER)..."; \
		useradd --system --no-create-home --shell /usr/sbin/nologin --user-group $(LLAMACPP_SERVICE_USER); \
		EXTRA_GROUPS=""; \
		getent group video  >/dev/null 2>&1 && EXTRA_GROUPS="video"; \
		getent group render >/dev/null 2>&1 && EXTRA_GROUPS="$${EXTRA_GROUPS:+$$EXTRA_GROUPS,}render"; \
		if [ -n "$$EXTRA_GROUPS" ]; then usermod -aG "$$EXTRA_GROUPS" $(LLAMACPP_SERVICE_USER); fi; \
		echo "  created $(LLAMACPP_SERVICE_USER) (groups: $${EXTRA_GROUPS:-none})"; \
	fi

# Creates the default GGUF models directory (LLAMACPP_MODELS_DIR). Unlike a
# path under a user's home directory, /opt is world-traversable by default
# (see /opt's own 755 perms), so the llama-server service user can read
# whatever ends up here without any extra group membership or ACL — nothing
# to grant, unlike a home-directory path where the home dir's own perms
# (e.g. 750) commonly block traversal for a service account regardless of
# how open the models dir itself is. Idempotent (plain mkdir -p); requires
# root. Move your existing models in yourself, e.g.:
#   sudo mv /home/mpeters/models/*.gguf /home/mpeters/models/preset.ini $(LLAMACPP_MODELS_DIR)/
llamacpp-models-dir:
	@if [ "$$(id -u)" != "0" ]; then echo "requires root — run: sudo make llamacpp-models-dir" >&2; exit 1; fi
	install -d -m 0755 "$(LLAMACPP_MODELS_DIR)"
	@echo "models directory ready at $(LLAMACPP_MODELS_DIR) — move your .gguf files (and preset.ini, if any) in, then point LLAMA_ARG_MODELS_DIR/LLAMA_ARG_MODELS_PRESET at it in $(LLAMACPP_ENV_FILE)"

# This target owns deploying the SERVICE end to end, not just the unit file
# around it — that includes the binary the unit's ExecStart actually runs
# (llamacpp-install, which pulls in llamacpp-build/-clone), the account it
# runs as, and the models directory its env file points at. All of those are
# constituent parts of "the service exists and can start," not separable
# lifecycle stages a caller has to sequence themselves — the earlier version
# of this target treated "build the software" as decoupled from "set up the
# service," which is exactly how it ended up installed and enabled while
# pointing at a prefix that didn't exist yet (exit 127, restart-looping).
#
# In practice a rerun is cheap even with llamacpp-install chained in: cmake's
# own incremental build (llamacpp-build's `cmake --build`) finishes in well
# under a second when nothing changed. The one real cost: since this whole
# recipe runs as root (needs sudo for /opt, /etc, the service user), the
# clone/build steps now also run as root, so if LLAMACPP_SRC_DIR is a
# personal checkout you also hack on directly (not a dedicated clone), any
# files a rebuild touches become root-owned. Run `make llamacpp-build`
# yourself (unprivileged) first if you want to keep that tree entirely
# user-owned; this target's own incremental rerun will then be a no-op there.
#
# Seeds (never overwrites) the env file — same non-clobbering convention
# `make seed` uses for agentx.toml — with LLAMACPP_PREFIX/LLAMA_ARG_MODELS_DIR
# substituted from the ACTUAL LLAMACPP_INSTALL_PREFIX/LLAMACPP_MODELS_DIR
# values used here, not just a copy of the template's own hardcoded
# defaults, so it stays correct even if you overrode either one. Does NOT
# enable or start the service.
llamacpp-service-install: llamacpp-install llamacpp-service-user llamacpp-models-dir
	install -d "$(LLAMACPP_SERVICE_LIBDIR)"
	install -m 0755 scripts/llama-server "$(LLAMACPP_SERVICE_LIBDIR)/llama-server"
	install -m 0644 scripts/llama-server.service "$(LLAMACPP_UNIT_FILE)"
	@if [ -e "$(LLAMACPP_ENV_FILE)" ]; then \
		echo "  skip  $(LLAMACPP_ENV_FILE) (already exists — edit it directly to change settings)"; \
	else \
		install -m 0640 -o root -g $(LLAMACPP_SERVICE_USER) scripts/llama-server.env.example "$(LLAMACPP_ENV_FILE)"; \
		sed -i \
			-e 's|^LLAMACPP_PREFIX=.*|LLAMACPP_PREFIX=$(LLAMACPP_INSTALL_PREFIX)|' \
			-e 's|^LLAMA_ARG_MODELS_DIR=.*|LLAMA_ARG_MODELS_DIR=$(LLAMACPP_MODELS_DIR)|' \
			"$(LLAMACPP_ENV_FILE)"; \
		echo "  seed  $(LLAMACPP_ENV_FILE) (LLAMACPP_PREFIX=$(LLAMACPP_INSTALL_PREFIX), LLAMA_ARG_MODELS_DIR=$(LLAMACPP_MODELS_DIR))"; \
	fi
	systemctl daemon-reload
	@echo ""
	@echo "Installed. $(LLAMACPP_ENV_FILE) already points at $(LLAMACPP_INSTALL_PREFIX) / $(LLAMACPP_MODELS_DIR) if freshly seeded — edit it only if you need something else (e.g. LLAMA_ARG_MODELS_PRESET), then:"
	@echo "  sudo systemctl enable --now llama-server"
	@echo "  journalctl -u llama-server -f"

# Removes the unit + wrapper script. Leaves $(LLAMACPP_ENV_FILE) and the
# $(LLAMACPP_SERVICE_USER) system user in place — the env file may hold
# tuned settings worth keeping, and removing a system user by hand is a more
# deliberate action than this target should take on your behalf.
llamacpp-service-uninstall:
	@if [ "$$(id -u)" != "0" ]; then echo "requires root — run: sudo make llamacpp-service-uninstall" >&2; exit 1; fi
	-systemctl disable --now llama-server
	rm -f "$(LLAMACPP_UNIT_FILE)"
	rm -f "$(LLAMACPP_SERVICE_LIBDIR)/llama-server"
	systemctl daemon-reload
	@echo "llama-server.service removed. $(LLAMACPP_ENV_FILE) and the $(LLAMACPP_SERVICE_USER) user left intact."
