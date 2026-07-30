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
//
// Transactional writes (Phase 1b, see docs/implementation/03_configuration_and_storage.md):
//
//	GIVEN a deployment config path and a Config value
//	WHEN  WriteConfig is called
//	THEN  the config-cache semaphore (~/.cache/agentx/config.lock) is acquired,
//	      cfg is encoded to a timestamped temp file (~/.cache/agentx/config_<ms>.tmp),
//	      and that temp is atomically renamed onto the deployment path.
//	      The lock is released after the rename completes (or on error, via defer).
//
//	GIVEN multiple concurrent writers
//	WHEN  they call WriteConfig in parallel
//	THEN  at most one writer holds the lock at a time; the others block until
//	      the lock is released (syscall.Flock advisory lock).
//
//	GIVEN a stale temp file left by a crashed writer
//	WHEN  the orchestrator starts up
//	THEN  CleanupStaleTemps purges config_*.tmp files from the cache dir so the
//	      cache dir does not accumulate orphan temps across restarts.
//
// The atomic rename fallback handles the cross-device case (temp in cache dir,
// deployment on a different filesystem): copy to dst's parent dir, then rename
// locally. Same atomic guarantee on the local fs.
package config
