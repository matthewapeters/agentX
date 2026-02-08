"""Live performance benchmarks for AgentX + Agentix.

These tests require a running Ollama instance and are skipped by default.
Enable with: AGENTIX_BENCH_RUN=1 pytest -m live tests/integration/test_performance.py
"""

import os
import sys
import time
from pathlib import Path

import pytest

# Ensure shared prompts resolve correctly
PROJECT_ROOT = Path(__file__).resolve().parents[2]
os.environ["AGENTIX_HOME"] = str(PROJECT_ROOT)

# Add src to path
sys.path.insert(0, str(PROJECT_ROOT / "src"))

from agentix.agentix_config import AgentixConfig
from agentix.bridge import AgentixBridge
from shared.models.context import Context
from shared.models.message import user_message
from shared.models.response import ChunkType


def _bench_enabled() -> bool:
    return os.getenv("AGENTIX_BENCH_RUN") == "1"


def _create_bridge() -> AgentixBridge:
    config = AgentixConfig(
        model=os.getenv("AGENTIX_BENCH_MODEL", "gpt-oss"),
        temperature=0.2,
        ollama_host=os.getenv("AGENTIX_BENCH_OLLAMA_HOST", "localhost:11434"),
        debug=False,
    )
    config.classification_model = os.getenv("AGENTIX_BENCH_CLASSIFICATION_MODEL")
    classify_max_tokens = os.getenv("AGENTIX_BENCH_CLASSIFY_MAX_TOKENS")
    if classify_max_tokens:
        try:
            config.classification_max_tokens = int(classify_max_tokens)
        except ValueError:
            pytest.fail(
                "AGENTIX_BENCH_CLASSIFY_MAX_TOKENS must be an integer"
            )
    bridge = AgentixBridge(config)

    try:
        models = bridge.get_available_models()
    except Exception as exc:
        pytest.skip(f"Ollama not available: {exc}")

    if not models:
        pytest.skip("No Ollama models found")

    if config.model:
        matching = [m for m in models if m.get("name", "").startswith(config.model)]
        if not matching:
            pytest.skip(f"Model '{config.model}' not found in Ollama")
        config.model = matching[0].get("name")
    else:
        config.model = models[0].get("name")

    return bridge


@pytest.mark.live
def test_classification_latency():
    """Classification should complete within the configured threshold."""
    if not _bench_enabled():
        pytest.skip("Set AGENTIX_BENCH_RUN=1 to enable benchmarks")

    bridge = _create_bridge()
    context = Context()
    context.add_message(user_message("Hello"))

    max_seconds = float(os.getenv("AGENTIX_BENCH_CLASSIFY_MAX_SECONDS", "2.0"))
    start = time.monotonic()
    classification = bridge.classify_prompt("what model are you?", context)
    elapsed = time.monotonic() - start

    assert classification is not None
    assert elapsed < max_seconds, (
        f"Classification took {elapsed:.2f}s, exceeds {max_seconds:.2f}s"
    )


@pytest.mark.live
def test_streaming_first_chunk_latency():
    """Streaming should yield a content chunk quickly."""
    if not _bench_enabled():
        pytest.skip("Set AGENTIX_BENCH_RUN=1 to enable benchmarks")

    bridge = _create_bridge()
    bridge.config.classify_prompts = False

    context = Context()
    max_seconds = float(
        os.getenv("AGENTIX_BENCH_STREAM_FIRST_CHUNK_MAX_SECONDS", "8.0")
    )

    start = time.monotonic()
    first_chunk_time = None

    for chunk in bridge.process_prompt_streaming("Say hello.", context):
        if chunk.chunk_type == ChunkType.ERROR:
            pytest.fail(f"Streaming error: {chunk.content}")
        if chunk.content:
            first_chunk_time = time.monotonic() - start
            break

    assert first_chunk_time is not None, "No content chunk received"
    assert first_chunk_time < max_seconds, (
        f"First chunk took {first_chunk_time:.2f}s, exceeds {max_seconds:.2f}s"
    )
