"""
Tests for Message.to_llm_dict() Ollama/OpenAI wire format.

Covers the TOOL_CALL and TOOL_RESULT serialization that feeds the agentic
tool loop, and the Context helper methods added in Phase 4.1.
"""

import json
import pytest
from shared.models.message import (
    Message,
    MessageRole,
    tool_call_message,
    tool_result_message,
    user_message,
    assistant_message,
    system_message,
    thinking_message,
)
from shared.models.context import Context


# ---------------------------------------------------------------------------
# to_llm_dict — standard roles
# ---------------------------------------------------------------------------


class TestStandardRoleWireFormat:
    def test_user_message(self):
        msg = user_message("hello")
        d = msg.to_llm_dict()
        assert d == {"role": "user", "content": "hello"}

    def test_assistant_message(self):
        msg = assistant_message("world")
        d = msg.to_llm_dict()
        assert d == {"role": "assistant", "content": "world"}

    def test_system_message(self):
        msg = system_message("you are helpful")
        d = msg.to_llm_dict()
        assert d == {"role": "system", "content": "you are helpful"}

    def test_thinking_message_maps_to_assistant(self):
        msg = thinking_message("reasoning...")
        d = msg.to_llm_dict()
        assert d["role"] == "assistant"
        assert d["content"] == "reasoning..."


# ---------------------------------------------------------------------------
# to_llm_dict — TOOL_CALL wire format
# ---------------------------------------------------------------------------


class TestToolCallWireFormat:
    def test_role_is_assistant(self):
        msg = tool_call_message("get_ast", {"file": "foo.py"}, tool_id="call_1")
        d = msg.to_llm_dict()
        assert d["role"] == "assistant"

    def test_tool_calls_array_present(self):
        msg = tool_call_message("get_ast", {"file": "foo.py"}, tool_id="call_1")
        d = msg.to_llm_dict()
        assert "tool_calls" in d
        assert len(d["tool_calls"]) == 1

    def test_tool_call_id_preserved(self):
        msg = tool_call_message("get_ast", {"file": "foo.py"}, tool_id="call_abc")
        d = msg.to_llm_dict()
        assert d["tool_calls"][0]["id"] == "call_abc"

    def test_tool_call_type_is_function(self):
        msg = tool_call_message("get_ast", {})
        d = msg.to_llm_dict()
        assert d["tool_calls"][0]["type"] == "function"

    def test_function_name_and_arguments(self):
        args = {"file": "foo.py", "line": 10}
        msg = tool_call_message("get_ast", args, tool_id="call_x")
        d = msg.to_llm_dict()
        fn = d["tool_calls"][0]["function"]
        assert fn["name"] == "get_ast"
        # arguments must be a JSON string
        parsed = json.loads(fn["arguments"])
        assert parsed == args

    def test_missing_tool_id_defaults(self):
        msg = tool_call_message("my_tool", {})
        d = msg.to_llm_dict()
        assert d["tool_calls"][0]["id"] == "call_0"

    def test_no_content_key_interference(self):
        """TOOL_CALL must not produce a user-facing [Tool Call: ...] string."""
        msg = tool_call_message("my_tool", {"x": 1})
        d = msg.to_llm_dict()
        assert "[Tool Call:" not in d.get("content", "")


# ---------------------------------------------------------------------------
# to_llm_dict — TOOL_RESULT wire format
# ---------------------------------------------------------------------------


class TestToolResultWireFormat:
    def test_role_is_tool(self):
        msg = tool_result_message(tool_id="call_1", tool_name="get_ast", tool_output="result")
        d = msg.to_llm_dict()
        assert d["role"] == "tool"

    def test_tool_call_id_preserved(self):
        msg = tool_result_message(tool_id="call_xyz", tool_name="get_ast", tool_output="ok")
        d = msg.to_llm_dict()
        assert d["tool_call_id"] == "call_xyz"

    def test_string_output(self):
        msg = tool_result_message(tool_id="c1", tool_name="t", tool_output="hello")
        d = msg.to_llm_dict()
        assert d["content"] == "hello"

    def test_dict_output_serialized_to_json(self):
        output = {"lines": 42, "name": "foo"}
        msg = tool_result_message(tool_id="c1", tool_name="t", tool_output=output)
        d = msg.to_llm_dict()
        assert json.loads(d["content"]) == output

    def test_list_output_serialized_to_json(self):
        output = [1, 2, 3]
        msg = tool_result_message(tool_id="c1", tool_name="t", tool_output=output)
        d = msg.to_llm_dict()
        assert json.loads(d["content"]) == output

    def test_none_output_uses_content_field(self):
        msg = Message(
            role=MessageRole.TOOL_RESULT,
            content="fallback text",
            tool_id="c1",
        )
        d = msg.to_llm_dict()
        assert d["content"] == "fallback text"

    def test_missing_tool_id_defaults(self):
        msg = tool_result_message(tool_name="t", tool_output="x")
        d = msg.to_llm_dict()
        assert d["tool_call_id"] == "call_0"

    def test_no_tool_role_in_standard_messages(self):
        """Sanity-check: regular user message must NOT produce role=tool."""
        msg = user_message("hi")
        assert msg.to_llm_dict()["role"] != "tool"


# ---------------------------------------------------------------------------
# Context helpers
# ---------------------------------------------------------------------------


class TestContextToolHelpers:
    def test_add_tool_call_message_returns_message(self):
        ctx = Context()
        msg = ctx.add_tool_call_message("my_tool", {"a": 1}, tool_id="c1")
        assert msg.role == MessageRole.TOOL_CALL
        assert msg.tool_name == "my_tool"
        assert msg.tool_id == "c1"

    def test_add_tool_call_message_appended_to_context(self):
        ctx = Context()
        ctx.add_tool_call_message("my_tool", {})
        assert len(ctx.messages) == 1
        assert ctx.messages[0].message.role == MessageRole.TOOL_CALL

    def test_add_tool_result_message_returns_message(self):
        ctx = Context()
        msg = ctx.add_tool_result_message("my_tool", "output", tool_id="c1")
        assert msg.role == MessageRole.TOOL_RESULT
        assert msg.tool_id == "c1"

    def test_add_tool_result_message_appended(self):
        ctx = Context()
        ctx.add_tool_result_message("my_tool", {"k": "v"})
        assert len(ctx.messages) == 1
        assert ctx.messages[0].message.role == MessageRole.TOOL_RESULT

    def test_to_llm_messages_round_trip(self):
        """Full round-trip: context helpers → to_llm_messages() → correct wire format."""
        ctx = Context()
        ctx.add_message(user_message("what is 2+2?"))
        ctx.add_tool_call_message("calculator", {"expr": "2+2"}, tool_id="c1")
        ctx.add_tool_result_message("calculator", "4", tool_id="c1")

        msgs = ctx.to_llm_messages()
        assert msgs[0] == {"role": "user", "content": "what is 2+2?"}
        assert msgs[1]["role"] == "assistant"
        assert msgs[1]["tool_calls"][0]["id"] == "c1"
        assert msgs[2]["role"] == "tool"
        assert msgs[2]["tool_call_id"] == "c1"
        assert msgs[2]["content"] == "4"
