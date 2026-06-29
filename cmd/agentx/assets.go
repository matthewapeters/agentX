package main

import _ "embed"

// logo is the application logo banner, embedded from cmd/agentx/assets/agentx.logo.
// The Makefile keeps that copy in sync with the authored source logo/agentx.logo
// (see docs/implementation/09_makefile_and_quality_gate_contract.md). It is shown
// as the first element of the chat output surface at bootstrap.
//
//go:embed assets/agentx.logo
var logo string
