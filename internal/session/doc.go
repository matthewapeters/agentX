// Package session manages session identity and the on-disk session store.
//
// Source contract: docs/implementation/03_configuration_and_storage.md
// (Session Storage Root) and docs/implementation/01_runtime_blueprint.md
// (Session identity policy). Backlog task: CHT-A2.
//
// Identity (GIVEN/WHEN/THEN):
//
//	GIVEN a session root directory
//	WHEN  a session is created
//	THEN  it receives a canonical session_id (epoch-derived, dir-unique) and a
//	      human-readable session_name (adjective-noun) that is unique within the
//	      root, resolving collisions with a deterministic numeric suffix, and its
//	      directory (with an events/ folder and session.json metadata) is created.
//
// Event persistence (CHT-A4) is layered on the same store.
package session
