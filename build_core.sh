#!/bin/bash
# Build script for Go core executable.

set -e

echo "Building AgentX Go core..."
cd "$(dirname "$0")/cmd/agentx-core"

# Download dependencies
echo "Downloading Go dependencies..."
go mod download

# Build binary
echo "Building binary..."
go build -o ../../bin/agentx

echo "✓ Build complete: bin/agentx"
ls -lh ../../bin/agentx
