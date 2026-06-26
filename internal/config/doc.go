// Package config resolves the effective AgentX runtime configuration.
//
// Source contract: docs/implementation/03_configuration_and_storage.md.
// Backlog task: CHT-A1 (docs/build-plan/03_chat_surface_backlog.md).
//
// Resolution behavior (GIVEN/WHEN/THEN):
//
//	GIVEN a deployment config at ~/.config/agentx/agentx.toml
//	WHEN  configuration is resolved
//	THEN  the deployment config is loaded over built-in defaults and returned.
//
//	GIVEN no deployment config, and a project config at <cwd>/.agentx/.agentx.toml
//	WHEN  configuration is resolved (first run)
//	THEN  built-in defaults are overlaid with the project config, the result is
//	      seeded to the deployment path, and returned with source "seeded".
//
//	GIVEN neither config exists
//	WHEN  configuration is resolved (first run)
//	THEN  built-in defaults are seeded to the deployment path and returned.
//
// Paths are injectable (see Paths/Resolve) so callers and tests can supply
// alternate locations; DefaultPaths derives the conventional locations, honoring
// XDG_CONFIG_HOME.
package config
