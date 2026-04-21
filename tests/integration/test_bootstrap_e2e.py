"""
End-to-end bootstrap tests for AgentX.

These tests require a live Ollama instance and are excluded from unit-test
runs via the ``live`` marker.  Enable with::

    AGENTIX_BENCH_RUN=1 pytest -m live tests/integration/test_bootstrap_e2e.py -v

What is verified
----------------
1. Default configuration is loaded as expected (models, prompts dir, flags).
2. The bootstrap prompt ("Hi! Identify yourself!") is classified as
   ``conversation`` / ``respond_directly`` with no clarification needed.
3. The LLM response identifies itself as "AgentX" (case-insensitive).
4. No ERROR-level log records are emitted by the agentix/agentx namespaces
   during the full sequence.
5. Classification completes within the acceptable latency threshold.
6. The full bootstrap sequence (classify + first content chunk) completes
   within the acceptable total latency threshold.

Timing thresholds (can be overridden via environment variables)
---------------------------------------------------------------
AGENTIX_BENCH_CLASSIFY_MAX_SECONDS   – max seconds for classification (default 5)
AGENTIX_BENCH_BOOTSTRAP_MAX_SECONDS  – max seconds for full sequence  (default 15)
"""

import logging
import os
import sys
import time
from pathlib import Path
from typing import Optional

import pytest

PROJECT_ROOT = Path(__file__).resolve().parents[2]
os.environ.setdefault("AGENTIX_HOME", str(PROJECT_ROOT))
sys.path.insert(0, str(PROJECT_ROOT / "src"))

from agentix.agentix_config import AgentixConfig
from agentix.bridge import AgentixBridge
from shared.models.context import Context
from shared.models.message import user_message
from shared.models.response import ChunkType

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------


def _bench_enabled() -> bool:
    return os.getenv("AGENTIX_BENCH_RUN") == "1"


def _load_toml() -> dict:
    """Load agentx.toml; returns empty dict if unavailable."""
    config_path = PROJECT_ROOT / "agentx.toml"
    if not config_path.exists():
        return {}
    try:
        import tomllib  # Python 3.11+
    except ImportError:
        import tomli as tomllib  # type: ignore[no-redef]
    with open(config_path, "rb") as fh:
        return tomllib.load(fh)


def _bootstrap_prompt() -> str:
    """Return the contents of .agentx/bootstrap-prompt.md."""
    prompt_path = PROJECT_ROOT / ".agentx" / "bootstrap-prompt.md"
    if prompt_path.is_file():
        text = prompt_path.read_text(encoding="utf-8").strip()
        if text:
            return text
    return "Hi!  Identify yourself!"


def _agentx_instructions() -> Optional[str]:
    """Return .agentx/agentx-instructions.md contents, or None if absent."""
    path = PROJECT_ROOT / ".agentx" / "agentx-instructions.md"
    if path.is_file():
        return path.read_text(encoding="utf-8")
    return None


def _make_bootstrap_context() -> Context:
    """Build a Context that mirrors what _build_shared_context() produces.

    The real session injects the agentx-instructions into working memory, which
    is then serialised as a SYSTEM message at the head of shared_context.  We
    replicate that here so the LLM receives its AgentX identity during the
    bootstrap streaming call.
    """
    from shared.models.message import Message, MessageRole

    context = Context()
    instructions = _agentx_instructions()
    if instructions:
        # Mirror: WorkingMemory.to_llm_block() → system message in shared_context
        wm_block = f"<working_memory>\n👤 agentx-instructions: {instructions}\n</working_memory>"
        sys_msg = Message(role=MessageRole.SYSTEM, content=wm_block)
        context.add_message(sys_msg)
    return context


def _create_bridge(toml: dict) -> AgentixBridge:
    """Build an AgentixBridge from toml config; skip if Ollama unavailable."""
    agentx = toml.get("agentx", {})
    agentix = toml.get("agentix", {})

    config = AgentixConfig(
        model=agentx.get("ollama_model", "gpt-oss"),
        temperature=agentx.get("temperature", 0.2),
        ollama_host=agentx.get("ollama_host", "localhost:11434"),
        classify_prompts=agentix.get("classify_prompts", True),
        classification_model=agentix.get("agentix_bench_classification_model"),
        debug=False,
        system_prompts_dir=str(PROJECT_ROOT / agentix.get("system_prompts_dir", "system_prompts")),
    )

    bridge = AgentixBridge(config)

    try:
        models = bridge.get_available_models()
    except Exception as exc:
        pytest.skip(f"Ollama not reachable: {exc}")

    if not models:
        pytest.skip("No Ollama models found")

    # Resolve full model tag (e.g. "gpt-oss" → "gpt-oss:latest")
    if config.model:
        match = [m for m in models if m.get("name", "").startswith(config.model)]
        if not match:
            pytest.skip(f"Chat model '{config.model}' not found in Ollama")
        config.model = match[0]["name"]

    if config.classification_model:
        cm = config.classification_model
        match = [m for m in models if m.get("name", "").startswith(cm)]
        if not match:
            pytest.skip(f"Classification model '{cm}' not found in Ollama")
        config.classification_model = match[0]["name"]

    return bridge


# ---------------------------------------------------------------------------
# Fixtures
# ---------------------------------------------------------------------------


@pytest.fixture(scope="module")
def toml_config() -> dict:
    return _load_toml()


@pytest.fixture(scope="module")
def bridge(toml_config) -> AgentixBridge:
    if not _bench_enabled():
        pytest.skip("Set AGENTIX_BENCH_RUN=1 to enable bootstrap e2e tests")
    return _create_bridge(toml_config)


# ---------------------------------------------------------------------------
# Tests
# ---------------------------------------------------------------------------


@pytest.mark.live
class TestBootstrapDefaults:
    """Verify that agentx.toml contains the expected default configuration."""

    def test_chat_model_is_configured(self, toml_config: dict) -> None:
        """Verify the agentx.toml specifies a chat model.

        GIVEN agentx.toml is present in the project root
        WHEN the agentx.ollama_model key is read
        THEN it must be non-empty and contain 'gpt-oss'
        """
        agentx_cfg = toml_config.get("agentx", {})
        model = agentx_cfg.get("ollama_model", "")
        assert model, "agentx.ollama_model must be set in agentx.toml"
        assert "gpt-oss" in model, f"Expected chat model to start with 'gpt-oss', got '{model}'"

    def test_classification_model_is_configured(self, toml_config: dict) -> None:
        """Verify a dedicated classification model is configured.

        GIVEN agentx.toml is present in the project root
        WHEN the agentix.agentix_bench_classification_model key is read
        THEN it must be non-empty and contain 'phi4-mini' so that
             the classification call does not use the agent's persona model
        """
        cm = toml_config.get("agentix", {}).get("agentix_bench_classification_model", "")
        assert cm, (
            "agentix.agentix_bench_classification_model must be set in agentx.toml "
            "so that classification uses a neutral model instead of the agent model"
        )
        assert "phi4-mini" in cm, f"Expected classification model to be 'phi4-mini:3.8b', got '{cm}'"

    def test_system_prompts_dir_exists(self, toml_config: dict) -> None:
        """Verify the system_prompts directory is present on disk.

        GIVEN agentx.toml specifies a system_prompts_dir
        WHEN the resolved path is checked on the filesystem
        THEN the directory must exist so PromptLoader can find prompt files
        """
        agentx_cfg = toml_config.get("agentx", {})
        prompts_dir = PROJECT_ROOT / agentx_cfg.get("system_prompts_dir", "system_prompts")
        assert prompts_dir.is_dir(), f"system_prompts_dir not found at '{prompts_dir}'"

    def test_classification_prompt_file_exists(self, toml_config: dict) -> None:
        """Verify prompt_classification.* exists in system_prompts_dir.

        GIVEN the system_prompts_dir is configured
        WHEN PromptLoader globs for 'prompt_classification.*'
        THEN at least one match must exist so the classification model
             receives its JSON-schema instruction instead of an empty system block
        """
        agentx_cfg = toml_config.get("agentx", {})
        prompts_dir = PROJECT_ROOT / agentx_cfg.get("system_prompts_dir", "system_prompts")
        candidates = list(prompts_dir.glob("prompt_classification.*"))
        assert candidates, (
            f"No 'prompt_classification.*' file found in '{prompts_dir}'. "
            "The classification system prompt is missing."
        )

    def test_bootstrap_prompt_file_exists(self) -> None:
        """Verify .agentx/bootstrap-prompt.md is present and non-empty.

        GIVEN the project root contains a .agentx directory
        WHEN bootstrap-prompt.md is read
        THEN it must exist and contain non-whitespace text
        """
        prompt_path = PROJECT_ROOT / ".agentx" / "bootstrap-prompt.md"
        assert prompt_path.is_file(), f"Bootstrap prompt not found at '{prompt_path}'"
        assert prompt_path.read_text(encoding="utf-8").strip(), "Bootstrap prompt file is empty"

    def test_agentx_instructions_file_exists_and_mentions_agentx(self) -> None:
        """Verify .agentx/agentx-instructions.md provides the AgentX identity.

        GIVEN the project root contains a .agentx directory
        WHEN agentx-instructions.md is read
        THEN it must exist and contain the word 'agentx' so the LLM
             adopts the correct identity when the instructions are injected
             into working memory at session start
        """
        instructions_path = PROJECT_ROOT / ".agentx" / "agentx-instructions.md"
        assert instructions_path.is_file(), (
            f"AgentX instructions not found at '{instructions_path}'. "
            "This file provides the AgentX identity injected into working memory at startup."
        )
        text = instructions_path.read_text(encoding="utf-8").lower()
        assert "agentx" in text, "agentx-instructions.md must mention 'AgentX' so the model knows its name"

    def test_classify_prompts_defaults_to_true(self, toml_config: dict) -> None:
        """Verify classify_prompts is True (default when key absent from toml).

        GIVEN agentx.toml may or may not contain classify_prompts
        WHEN the agentix section is read with a default of True
        THEN the resolved value must be True so every user prompt is
             classified before being routed
        """
        agentix = toml_config.get("agentix", {})
        # If the key is explicitly set, honour it; otherwise confirm the
        # adapter default kicks in (True).
        value = agentix.get("classify_prompts", True)
        assert value is True, f"classify_prompts should be True by default, got {value!r}"


@pytest.mark.live
class TestBootstrapClassification:
    """Bootstrap prompt must be classified correctly within latency budget."""

    def test_bootstrap_prompt_intent_is_conversation(
        self, bridge: AgentixBridge, caplog: pytest.LogCaptureFixture
    ) -> None:
        """Bootstrap greeting is classified as conversation/respond_directly.

        GIVEN a live Ollama instance with the configured classification model
        WHEN classify_prompt is called with the bootstrap greeting prompt
        THEN intent must be 'conversation'
         AND next_step must be 'respond_directly'
         AND needs_clarification must be False
         AND no ERROR-level log records must be emitted
         AND classification must complete within AGENTIX_BENCH_CLASSIFY_MAX_SECONDS
        """
        if not _bench_enabled():
            pytest.skip("Set AGENTIX_BENCH_RUN=1 to enable bootstrap e2e tests")

        prompt = _bootstrap_prompt()
        context = Context()
        context.add_message(user_message(prompt))

        max_seconds = float(os.getenv("AGENTIX_BENCH_CLASSIFY_MAX_SECONDS", "5.0"))

        with caplog.at_level(logging.ERROR, logger="agentix.classification"):
            start = time.monotonic()
            result = bridge.classify_prompt(prompt, context)
            elapsed = time.monotonic() - start

        # --- correctness ---
        assert result is not None, "classify_prompt returned None"
        assert result.intent.name == "conversation", (
            f"Expected intent='conversation', got '{result.intent.name}'. " f"Reasoning: {result.reasoning_summary}"
        )
        assert (
            result.next_step.name == "respond_directly"
        ), f"Expected next_step='respond_directly', got '{result.next_step.name}'"
        assert result.needs_clarification is False, "Bootstrap prompt should not require clarification"
        assert not result.missing_fields, f"Unexpected missing_fields: {result.missing_fields}"

        # --- no errors ---
        errors = [r for r in caplog.records if r.levelno >= logging.ERROR]
        assert not errors, "ERROR-level log records during classification:\n" + "\n".join(
            f"  [{r.name}] {r.getMessage()}" for r in errors
        )

        # --- latency ---
        assert elapsed < max_seconds, f"Classification took {elapsed:.2f}s, exceeds budget of {max_seconds:.2f}s"


@pytest.mark.live
class TestBootstrapResponse:
    """LLM must identify itself as AgentX and emit no errors."""

    def test_response_identifies_as_agentx(self, bridge: AgentixBridge, caplog: pytest.LogCaptureFixture) -> None:
        """LLM streaming response identifies the assistant as AgentX.

        GIVEN a live Ollama instance with the configured chat model
         AND the agentx-instructions.md is injected as a system context message
        WHEN process_prompt_streaming is called with the bootstrap prompt
        THEN the concatenated response must contain 'agentx' (case-insensitive)
         AND no ERROR-level log records must be emitted
         AND no CHUNK_TYPE.ERROR chunks must appear in the stream
         AND the full response must complete within AGENTIX_BENCH_BOOTSTRAP_MAX_SECONDS
        """
        if not _bench_enabled():
            pytest.skip("Set AGENTIX_BENCH_RUN=1 to enable bootstrap e2e tests")

        prompt = _bootstrap_prompt()
        # Mirror the real app: inject agentx-instructions as a system message so
        # the model knows its AgentX identity (loaded from working memory in session.py)
        context = _make_bootstrap_context()

        max_total_seconds = float(os.getenv("AGENTIX_BENCH_BOOTSTRAP_MAX_SECONDS", "15.0"))

        content_parts: list[str] = []
        first_chunk_time: Optional[float] = None
        errors_in_stream: list[str] = []

        with caplog.at_level(logging.ERROR, logger="agentix"):
            start = time.monotonic()

            # Disable classify_prompts in the bridge for this call so we test
            # streaming in isolation (classification is covered above).
            original_classify = bridge.config.classify_prompts
            bridge.config.classify_prompts = False
            try:
                for chunk in bridge.process_prompt_streaming(prompt, context):
                    if chunk.chunk_type == ChunkType.ERROR:
                        errors_in_stream.append(chunk.content or "(no detail)")
                        break
                    if chunk.content:
                        if first_chunk_time is None:
                            first_chunk_time = time.monotonic() - start
                        content_parts.append(chunk.content)
            finally:
                bridge.config.classify_prompts = original_classify

        elapsed = time.monotonic() - start
        full_response = "".join(content_parts)

        # --- no stream errors ---
        assert not errors_in_stream, f"Stream returned ERROR chunk(s): {errors_in_stream}"

        # --- got content ---
        assert full_response.strip(), "No content received from LLM"

        # --- AgentX mentioned ---
        assert "agentx" in full_response.lower(), (
            f"LLM response does not mention 'AgentX'.\n"
            f"Full response ({len(full_response)} chars):\n{full_response[:500]}"
        )

        # --- no log errors ---
        log_errors = [r for r in caplog.records if r.levelno >= logging.ERROR]
        assert not log_errors, "ERROR-level log records during streaming:\n" + "\n".join(
            f"  [{r.name}] {r.getMessage()}" for r in log_errors
        )

        # --- first chunk latency ---
        assert first_chunk_time is not None, "No content chunk received"

        # --- total latency ---
        assert (
            elapsed < max_total_seconds
        ), f"Bootstrap response took {elapsed:.2f}s, exceeds budget of {max_total_seconds:.2f}s"


@pytest.mark.live
class TestBootstrapFullSequence:
    """Classify then stream — mirrors what _run_bootstrap_prompt_if_present does."""

    def test_full_bootstrap_sequence(self, bridge: AgentixBridge, caplog: pytest.LogCaptureFixture) -> None:
        """Full classify-then-stream pipeline mirrors _run_bootstrap_prompt_if_present.

        GIVEN a live Ollama instance with both the chat and classification models available
         AND the agentx-instructions are injected into the streaming context
        WHEN classify_prompt is called followed by process_prompt_streaming
        THEN classification intent must be 'conversation'
         AND the streamed response must mention 'AgentX'
         AND no ERROR-level log records must be emitted
         AND classification must complete within AGENTIX_BENCH_CLASSIFY_MAX_SECONDS
         AND the full sequence must complete within AGENTIX_BENCH_BOOTSTRAP_MAX_SECONDS
        """
        if not _bench_enabled():
            pytest.skip("Set AGENTIX_BENCH_RUN=1 to enable bootstrap e2e tests")

        prompt = _bootstrap_prompt()
        # Classify with just the user message (mirrors classify_prompt filtering)
        classify_context = Context()
        classify_context.add_message(user_message(prompt))
        # Stream with full context including AgentX instructions (mirrors shared_context)
        stream_context = _make_bootstrap_context()
        stream_context.add_message(user_message(prompt))

        max_total_seconds = float(os.getenv("AGENTIX_BENCH_BOOTSTRAP_MAX_SECONDS", "15.0"))

        with caplog.at_level(logging.ERROR):
            start = time.monotonic()

            # Step 1: classify (user message only — mirrors classify_prompt filtering)
            classification = bridge.classify_prompt(prompt, classify_context)
            classify_elapsed = time.monotonic() - start

            # Step 2: stream response (full context with identity instructions)
            content_parts: list[str] = []
            for chunk in bridge.process_prompt_streaming(prompt, stream_context, classification):
                if chunk.chunk_type == ChunkType.ERROR:
                    pytest.fail(f"Stream ERROR chunk: {chunk.content}")
                if chunk.content:
                    content_parts.append(chunk.content)

            total_elapsed = time.monotonic() - start

        full_response = "".join(content_parts)

        # correctness
        assert classification.intent.name == "conversation"
        assert classification.next_step.name == "respond_directly"
        assert "agentx" in full_response.lower(), f"Response does not mention AgentX:\n{full_response[:500]}"

        # no errors
        log_errors = [r for r in caplog.records if r.levelno >= logging.ERROR]
        assert not log_errors, "Errors during full bootstrap sequence:\n" + "\n".join(
            f"  [{r.name}] {r.getMessage()}" for r in log_errors
        )

        # latency
        classify_budget = float(os.getenv("AGENTIX_BENCH_CLASSIFY_MAX_SECONDS", "5.0"))
        assert (
            classify_elapsed < classify_budget
        ), f"Classification took {classify_elapsed:.2f}s (budget {classify_budget:.2f}s)"
        assert (
            total_elapsed < max_total_seconds
        ), f"Full bootstrap took {total_elapsed:.2f}s (budget {max_total_seconds:.2f}s)"
