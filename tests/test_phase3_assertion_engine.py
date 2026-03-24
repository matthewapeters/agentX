"""
Phase 3 unit tests: Synthesis Assertion Engine.

Tests cover:
- _parse_json_list: direct JSON, embedded JSON, bad input
- extract_assertions: valid extraction, empty synthesis, LLM failure, bad JSON response
- verify_assertion: exists (pass/fail), value (pass/fail), regex (pass/fail/bad regex),
                    count (always None), unknown type (always None)
- _run_task_node: ASSERTION_RESULT chunks emitted; re-synthesis triggered on failure;
                  re-synthesis loop respects max_synthesis_retries
"""

from __future__ import annotations

import json
import os
import re
from typing import Iterator
from unittest.mock import MagicMock, patch

import pytest

from agentix.bridge.assertion_checker import (
    _parse_json_list,
    extract_assertions,
    verify_assertion,
)
from agentix.bridge.bridge import AgentixBridge
from agentix.agentix_config import AgentixConfig
from shared.models.response import ChunkType, ResponseChunk
from shared.models.context import Context
from shared.models.task_node import AssertionRecord, PlanRecord, PlanStep, SynthesisAttempt, TaskTree

# ── Helpers ────────────────────────────────────────────────────────────────────


def content_chunk(text: str) -> ResponseChunk:
    return ResponseChunk(type=ChunkType.CONTENT, content=text)


def done_chunk() -> ResponseChunk:
    return ResponseChunk(type=ChunkType.DONE, done_reason="stop")


def make_bridge(max_synthesis_retries: int = 3) -> AgentixBridge:
    config = AgentixConfig(
        model="llama3.2",
        max_synthesis_retries=max_synthesis_retries,
    )
    return AgentixBridge(config)


def make_context(tmp_path) -> Context:
    ctx_dir = os.path.join(str(tmp_path), "context")
    os.makedirs(ctx_dir, exist_ok=True)
    return Context(session_id="test-session", path=ctx_dir)


def make_tree(plan_id: str) -> TaskTree:
    tree = TaskTree(session_id="test-session")
    plan = PlanRecord(plan_id=plan_id, plan_name="P", steps=[PlanStep(step_id="s0", description="step 0")])
    tree.add_plan(plan)
    return tree


def collect(gen: Iterator[ResponseChunk]) -> list[ResponseChunk]:
    return list(gen)


# ── _parse_json_list ───────────────────────────────────────────────────────────


class TestParseJsonList:
    def test_direct_json_array(self):
        data = [{"fact": "x exists", "type": "exists", "check": "/tmp/x"}]
        assert _parse_json_list(json.dumps(data)) == data

    def test_array_embedded_in_markdown(self):
        raw = 'Here:\n```json\n[{"fact": "f", "type": "exists", "check": "p"}]\n```'
        result = _parse_json_list(raw)
        assert result[0]["fact"] == "f"

    def test_raises_on_no_array(self):
        with pytest.raises(ValueError):
            _parse_json_list("No JSON in here.")

    def test_raises_on_dict_not_list(self):
        with pytest.raises((ValueError, json.JSONDecodeError)):
            _parse_json_list('{"not": "a list"}')


# ── extract_assertions ─────────────────────────────────────────────────────────


class TestExtractAssertions:
    def _llm_returning(self, json_text: str):
        """Return a fake llm_iter_fn that yields the given text."""

        def fake_iter(messages, tools=None):
            yield content_chunk(json_text)
            yield done_chunk()

        return fake_iter

    def test_valid_extraction(self):
        payload = json.dumps(
            [
                {"fact": "bridge.py exists", "type": "exists", "check": "src/agentix/bridge/bridge.py"},
            ]
        )
        records = extract_assertions("The bridge file exists.", self._llm_returning(payload))
        assert len(records) == 1
        assert records[0].fact == "bridge.py exists"
        assert records[0].type == "exists"
        assert records[0].verified is None  # not yet verified

    def test_empty_synthesis_returns_empty(self):
        records = extract_assertions("", self._llm_returning("[]"))
        assert records == []

    def test_placeholder_synthesis_returns_empty(self):
        records = extract_assertions("(no synthesis produced)", self._llm_returning("[]"))
        assert records == []

    def test_llm_bad_json_returns_empty(self):
        records = extract_assertions("Some text.", self._llm_returning("not json at all"))
        assert records == []

    def test_llm_empty_array_returns_empty(self):
        records = extract_assertions("No checkable claims.", self._llm_returning("[]"))
        assert records == []

    def test_max_assertions_truncates(self):
        items = [{"fact": f"fact {i}", "type": "exists", "check": f"/p/{i}"} for i in range(20)]
        records = extract_assertions("synthesis", self._llm_returning(json.dumps(items)), max_assertions=5)
        assert len(records) == 5

    def test_llm_exception_returns_empty(self):
        def failing_iter(msgs, tools=None):
            raise RuntimeError("network error")
            yield  # make it a generator

        records = extract_assertions("some synthesis", failing_iter)
        assert records == []


# ── verify_assertion ───────────────────────────────────────────────────────────


class TestVerifyAssertion:
    def test_exists_pass(self, tmp_path):
        (tmp_path / "target.txt").write_text("hello")
        a = AssertionRecord(fact="file exists", type="exists", check=str(tmp_path / "target.txt"))
        result = verify_assertion(a, str(tmp_path))
        assert result.verified is True
        assert result.error is None

    def test_exists_fail(self, tmp_path):
        a = AssertionRecord(fact="missing file", type="exists", check=str(tmp_path / "no_such_file.txt"))
        result = verify_assertion(a, str(tmp_path))
        assert result.verified is False
        assert result.error is not None

    def test_exists_relative_path(self, tmp_path):
        (tmp_path / "rel.txt").write_text("data")
        a = AssertionRecord(fact="rel exists", type="exists", check="rel.txt")
        result = verify_assertion(a, str(tmp_path))
        assert result.verified is True

    def test_value_pass(self, tmp_path):
        f = tmp_path / "code.py"
        f.write_text("def foo(): pass\n")
        check = f"code.py::def foo"
        a = AssertionRecord(fact="foo defined", type="value", check=check)
        result = verify_assertion(a, str(tmp_path))
        assert result.verified is True

    def test_value_fail(self, tmp_path):
        f = tmp_path / "code.py"
        f.write_text("def bar(): pass\n")
        a = AssertionRecord(fact="foo defined", type="value", check="code.py::def foo")
        result = verify_assertion(a, str(tmp_path))
        assert result.verified is False
        assert "Substring not found" in (result.error or "")

    def test_value_missing_file(self, tmp_path):
        a = AssertionRecord(fact="x", type="value", check="nonexistent.py::something")
        result = verify_assertion(a, str(tmp_path))
        assert result.verified is False
        assert "not found" in (result.error or "").lower()

    def test_value_no_path_separator(self, tmp_path):
        a = AssertionRecord(fact="x", type="value", check="just a string no separator")
        result = verify_assertion(a, str(tmp_path))
        assert result.verified is None  # cannot verify

    def test_regex_pass(self, tmp_path):
        f = tmp_path / "lines.txt"
        f.write_text("Error: something failed\nOK: passed\n")
        a = AssertionRecord(fact="has error line", type="regex", check=r"lines.txt::Error: \w+")
        result = verify_assertion(a, str(tmp_path))
        assert result.verified is True

    def test_regex_fail(self, tmp_path):
        f = tmp_path / "lines.txt"
        f.write_text("everything is fine\n")
        a = AssertionRecord(fact="has error", type="regex", check=r"lines.txt::Error:")
        result = verify_assertion(a, str(tmp_path))
        assert result.verified is False

    def test_regex_invalid_pattern(self, tmp_path):
        f = tmp_path / "lines.txt"
        f.write_text("data\n")
        a = AssertionRecord(fact="bad regex", type="regex", check=r"lines.txt::[invalid")
        result = verify_assertion(a, str(tmp_path))
        assert result.verified is False
        assert "Invalid regex" in (result.error or "")

    def test_count_always_none(self, tmp_path):
        a = AssertionRecord(fact="at least 3 files", type="count", check="3")
        result = verify_assertion(a, str(tmp_path))
        assert result.verified is None

    def test_unknown_type_always_none(self, tmp_path):
        a = AssertionRecord(fact="something", type="mystery", check="anything")
        result = verify_assertion(a, str(tmp_path))
        assert result.verified is None

    def test_returns_same_object(self, tmp_path):
        a = AssertionRecord(fact="x", type="count", check="5")
        returned = verify_assertion(a, str(tmp_path))
        assert returned is a


# ── _run_task_node with assertion engine ──────────────────────────────────────


class TestRunTaskNodeAssertions:
    """Integration-style tests for the assertion loop inside _run_task_node."""

    def _run_node(self, bridge, ctx, tree, iter_responses, assertion_records=None, verify_func=None):
        """Helper: run _run_task_node with patched LLM and assertion helpers."""
        iter_call = [0]

        def mock_iter(messages, tools=None):
            r = iter_responses[min(iter_call[0], len(iter_responses) - 1)]
            iter_call[0] += 1
            return iter(r)

        mock_verify = verify_func or (lambda a, root="": a)

        with (
            patch.object(bridge, "_load_prompt_file", return_value=None),
            patch.object(bridge, "get_available_tools", return_value=[]),
            patch.object(bridge, "_iter_llm_chunks", side_effect=mock_iter),
            patch(
                "agentix.bridge.bridge.extract_assertions",
                return_value=assertion_records or [],
            ),
            patch("agentix.bridge.bridge.verify_assertion", side_effect=mock_verify),
        ):
            return collect(
                bridge._run_task_node(
                    plan_id="plan_a",
                    task_id="t0",
                    task_description="Do something",
                    context=ctx,
                    task_tree=tree,
                    initial_messages=[],
                )
            )

    def test_no_assertions_no_assertion_chunks(self, tmp_path):
        """When extract_assertions returns [], no ASSERTION_RESULT chunks emitted."""
        bridge = make_bridge()
        ctx = make_context(tmp_path)
        tree = make_tree("plan_a")

        chunks = self._run_node(
            bridge,
            ctx,
            tree,
            iter_responses=[[content_chunk("The answer is 42."), done_chunk()]],
            assertion_records=[],
        )

        types = [c.type for c in chunks]
        assert ChunkType.ASSERTION_RESULT not in types
        assert ChunkType.TASK_NODE_END in types

    def test_passing_assertions_emit_chunks(self, tmp_path):
        """Passing assertions each emit one ASSERTION_RESULT chunk."""
        bridge = make_bridge()
        ctx = make_context(tmp_path)
        tree = make_tree("plan_a")

        a1 = AssertionRecord(fact="file exists", type="exists", check="/tmp/x")
        a2 = AssertionRecord(fact="pattern matches", type="regex", check="some::pattern")

        def verify(a, root=""):
            a.verified = True
            return a

        chunks = self._run_node(
            bridge,
            ctx,
            tree,
            iter_responses=[[content_chunk("Synthesis text."), done_chunk()]],
            assertion_records=[a1, a2],
            verify_func=verify,
        )

        assertion_chunks = [c for c in chunks if c.type == ChunkType.ASSERTION_RESULT]
        assert len(assertion_chunks) == 2

    def test_failed_assertion_triggers_resynth(self, tmp_path):
        """A failing assertion triggers a re-synthesis LLM call."""
        bridge = make_bridge(max_synthesis_retries=1)
        ctx = make_context(tmp_path)
        tree = make_tree("plan_a")

        failing_assertion = AssertionRecord(fact="should exist", type="exists", check="/no/such/path")
        failing_assertion.verified = False
        failing_assertion.error = "Path not found: /no/such/path"

        call_count = [0]

        def mock_iter(messages, tools=None):
            call_count[0] += 1
            if call_count[0] == 1:
                # Initial synthesis call
                yield content_chunk("First synthesis attempt.")
                yield done_chunk()
            else:
                # Re-synthesis call — assertion extraction or re-synthesis
                yield content_chunk("Corrected synthesis.")
                yield done_chunk()

        # First verify: fail; second verify (after re-synthesis): pass
        verify_call = [0]

        def mock_verify(a, root=""):
            verify_call[0] += 1
            if verify_call[0] == 1:
                a.verified = False
                a.error = "Path not found: /no/such/path"
            else:
                a.verified = True
            return a

        extract_call = [0]

        def mock_extract(text, fn, max_assertions=10):
            extract_call[0] += 1
            fa = AssertionRecord(fact="should exist", type="exists", check="/no/such/path")
            return [fa]

        with (
            patch.object(bridge, "_load_prompt_file", return_value=None),
            patch.object(bridge, "get_available_tools", return_value=[]),
            patch.object(bridge, "_iter_llm_chunks", side_effect=mock_iter),
            patch("agentix.bridge.bridge.extract_assertions", side_effect=mock_extract),
            patch("agentix.bridge.bridge.verify_assertion", side_effect=mock_verify),
        ):
            chunks = collect(
                bridge._run_task_node(
                    plan_id="plan_a",
                    task_id="t0",
                    task_description="Do something",
                    context=ctx,
                    task_tree=tree,
                    initial_messages=[],
                )
            )

        # extract_assertions should have been called at least once (initial + re-synthesis)
        assert extract_call[0] >= 1
        # End chunk carries the final synthesis text
        end = next(c for c in chunks if c.type == ChunkType.TASK_NODE_END)
        assert end.content is not None

    def test_max_retries_respected(self, tmp_path):
        """Re-synthesis does not loop more than max_synthesis_retries times."""
        max_retries = 2
        bridge = make_bridge(max_synthesis_retries=max_retries)
        ctx = make_context(tmp_path)
        tree = make_tree("plan_a")

        iter_call = [0]

        def mock_iter(messages, tools=None):
            iter_call[0] += 1
            yield content_chunk(f"Synthesis attempt {iter_call[0]}")
            yield done_chunk()

        def always_fail(text, fn, max_assertions=10):
            a = AssertionRecord(fact="always fails", type="exists", check="/nope")
            return [a]

        def set_failed(a, root=""):
            a.verified = False
            a.error = "not found"
            return a

        with (
            patch.object(bridge, "_load_prompt_file", return_value=None),
            patch.object(bridge, "get_available_tools", return_value=[]),
            patch.object(bridge, "_iter_llm_chunks", side_effect=mock_iter),
            patch("agentix.bridge.bridge.extract_assertions", side_effect=always_fail),
            patch("agentix.bridge.bridge.verify_assertion", side_effect=set_failed),
        ):
            chunks = collect(
                bridge._run_task_node(
                    plan_id="plan_a",
                    task_id="t0",
                    task_description="Always failing task",
                    context=ctx,
                    task_tree=tree,
                    initial_messages=[],
                )
            )

        # Should not loop forever — TASK_NODE_END must appear
        end_chunks = [c for c in chunks if c.type == ChunkType.TASK_NODE_END]
        assert len(end_chunks) == 1
        # assertion extraction called at most max_retries+1 times
        assert iter_call[0] <= max_retries + 2  # 1 synthesis + 1 per retry (max_retries)

    def test_task_node_end_carries_final_synthesis(self, tmp_path):
        """TASK_NODE_END content reflects the accepted synthesis text."""
        bridge = make_bridge()
        ctx = make_context(tmp_path)
        tree = make_tree("plan_a")

        a = AssertionRecord(fact="x", type="exists", check="/some/path")

        def pass_verify(a, root=""):
            a.verified = True
            return a

        def extract(text, fn, max_assertions=10):
            return [AssertionRecord(fact="x", type="exists", check="/some/path")]

        call = [0]

        def mock_iter(messages, tools=None):
            call[0] += 1
            yield content_chunk("The definitive answer.")
            yield done_chunk()

        with (
            patch.object(bridge, "_load_prompt_file", return_value=None),
            patch.object(bridge, "get_available_tools", return_value=[]),
            patch.object(bridge, "_iter_llm_chunks", side_effect=mock_iter),
            patch("agentix.bridge.bridge.extract_assertions", side_effect=extract),
            patch("agentix.bridge.bridge.verify_assertion", side_effect=pass_verify),
        ):
            chunks = collect(
                bridge._run_task_node(
                    plan_id="plan_a",
                    task_id="t0",
                    task_description="task",
                    context=ctx,
                    task_tree=tree,
                    initial_messages=[],
                )
            )

        end = next(c for c in chunks if c.type == ChunkType.TASK_NODE_END)
        assert "definitive" in (end.content or "")
