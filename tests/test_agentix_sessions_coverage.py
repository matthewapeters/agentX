"""
Coverage uplift tests for src/agentix/context/sessions.py.

Targets the 19% → 90% uplift goal, covering all major code paths in:
  _get_user_name, _resolve_sessions_base, _ensure_session_context_dir,
  _get_latest_session_id, assemble_classification_prompt, assemble_prompts,
  trim_context, manage_sessions, update_session, get_session_history.
"""

import os
import pytest
from unittest.mock import MagicMock, patch

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------


def _make_config(**kwargs):
    from agentix.agentix_config import AgentixConfig
    from agentix.constants import DEFAULT_SESSION_ID

    cfg = AgentixConfig()
    cfg.session = kwargs.get("session", DEFAULT_SESSION_ID)
    cfg.model = kwargs.get("model", "test-model")
    cfg.classification_model = kwargs.get("classification_model", None)
    cfg.system = kwargs.get("system", [])
    cfg.tools = kwargs.get("tools", [])
    cfg.user = kwargs.get("user", [])
    cfg.file_path = kwargs.get("file_path", None)
    cfg.debug = kwargs.get("debug", False)
    cfg.temperature = kwargs.get("temperature", 0.7)
    cfg.classify_prompts = kwargs.get("classify_prompts", True)
    cfg.classification_max_tokens = kwargs.get("classification_max_tokens", None)
    return cfg


# ---------------------------------------------------------------------------
# _get_user_name
# ---------------------------------------------------------------------------


class TestGetUserName:
    def test_returns_USER_env(self):
        from agentix.context.sessions import _get_user_name

        with patch.dict(os.environ, {"USER": "testuser"}, clear=False):
            assert _get_user_name() == "testuser"

    def test_falls_back_to_USERNAME(self):
        from agentix.context.sessions import _get_user_name

        env = {k: v for k, v in os.environ.items() if k not in ("USER", "USERNAME")}
        env["USERNAME"] = "winuser"
        with patch.dict(os.environ, env, clear=True):
            assert _get_user_name() == "winuser"

    def test_falls_back_to_User_literal(self):
        from agentix.context.sessions import _get_user_name

        env = {k: v for k, v in os.environ.items() if k not in ("USER", "USERNAME")}
        with patch.dict(os.environ, env, clear=True):
            assert _get_user_name() == "User"


# ---------------------------------------------------------------------------
# _resolve_sessions_base
# ---------------------------------------------------------------------------


class TestResolveSessionsBase:
    def test_uses_env_var_when_set(self):
        from agentix.context.sessions import _resolve_sessions_base

        with patch.dict(os.environ, {"AGENTX_SESSIONS_DIR": "/tmp/custom_sessions"}):
            result = _resolve_sessions_base()
        assert result == "/tmp/custom_sessions"

    def test_uses_cwd_sessions_when_no_env(self):
        from agentix.context.sessions import _resolve_sessions_base

        env = {k: v for k, v in os.environ.items() if k != "AGENTX_SESSIONS_DIR"}
        with patch.dict(os.environ, env, clear=True):
            result = _resolve_sessions_base()
        assert result.endswith("sessions")
        assert os.getcwd() in result


# ---------------------------------------------------------------------------
# _ensure_session_context_dir
# ---------------------------------------------------------------------------


class TestEnsureSessionContextDir:
    def test_creates_context_dir_and_returns_path(self, tmp_path):
        from agentix.context.sessions import _ensure_session_context_dir

        cfg = _make_config(session="my-session")
        with patch.dict(os.environ, {"AGENTX_SESSIONS_DIR": str(tmp_path), "USER": "alice"}):
            result = _ensure_session_context_dir(cfg)
        assert os.path.isdir(result)
        assert result.endswith("context")

    def test_auto_generates_session_id_for_default(self, tmp_path):
        from agentix.context.sessions import _ensure_session_context_dir
        from agentix.constants import DEFAULT_SESSION_ID

        cfg = _make_config(session=DEFAULT_SESSION_ID)
        with patch.dict(os.environ, {"AGENTX_SESSIONS_DIR": str(tmp_path), "USER": "bob"}):
            _ensure_session_context_dir(cfg)
        # After calling, the session should no longer be the default placeholder
        assert cfg.session != DEFAULT_SESSION_ID
        assert cfg.session.startswith("session_")


# ---------------------------------------------------------------------------
# _get_latest_session_id
# ---------------------------------------------------------------------------


class TestGetLatestSessionId:
    def test_returns_none_when_user_dir_missing(self, tmp_path):
        from agentix.context.sessions import _get_latest_session_id

        result = _get_latest_session_id(str(tmp_path), "ghost_user")
        assert result is None

    def test_returns_none_when_no_session_dirs(self, tmp_path):
        from agentix.context.sessions import _get_latest_session_id

        user_dir = tmp_path / "alice"
        user_dir.mkdir()
        (user_dir / "not_a_session").mkdir()

        result = _get_latest_session_id(str(tmp_path), "alice")
        assert result is None

    def test_returns_latest_session(self, tmp_path):
        from agentix.context.sessions import _get_latest_session_id

        user_dir = tmp_path / "alice"
        user_dir.mkdir()
        (user_dir / "session_2024-01-01_10-00-00").mkdir()
        (user_dir / "session_2024-02-01_10-00-00").mkdir()

        result = _get_latest_session_id(str(tmp_path), "alice")
        assert result == "session_2024-02-01_10-00-00"


# ---------------------------------------------------------------------------
# trim_context
# ---------------------------------------------------------------------------


class TestTrimContext:
    def test_empty_history_returns_empty(self):
        from agentix.bridge.prompt_assembly import trim_context

        cfg = _make_config()
        result = trim_context(cfg, [], max_tokens=1000)
        assert result == []

    def test_system_messages_always_preserved(self):
        from agentix.bridge.prompt_assembly import trim_context

        cfg = _make_config()
        messages = [
            {"role": "system", "content": "You are helpful."},
            {"role": "user", "content": "A" * 4000},  # very large
        ]
        result = trim_context(cfg, messages, max_tokens=10)
        roles = [m["role"] for m in result]
        assert "system" in roles

    def test_older_messages_trimmed_when_over_limit(self):
        from agentix.bridge.prompt_assembly import trim_context

        cfg = _make_config()
        messages = [
            {"role": "user", "content": "old message " + "A" * 200},
            {"role": "user", "content": "new message"},
        ]
        result = trim_context(cfg, messages, max_tokens=10)
        # Only the most recent (short) message should survive
        contents = [m["content"] for m in result]
        assert any("new message" in c for c in contents)

    def test_message_with_null_content_is_handled(self):
        from agentix.bridge.prompt_assembly import trim_context

        cfg = _make_config()
        messages = [{"role": "user", "content": None}]
        # Should not raise
        result = trim_context(cfg, messages, max_tokens=1000)
        assert len(result) == 1

    def test_message_with_attachments_counted(self):
        from agentix.bridge.prompt_assembly import trim_context

        cfg = _make_config()
        msg = {
            "role": "user",
            "content": "short",
            "attachments": [{"content": "A" * 400}],
        }
        result = trim_context(cfg, [msg], max_tokens=10)
        # Attachment tokens push msg over limit → empty result
        assert result == []

    def test_non_dict_attachment_counted(self):
        """Covers the len(attachment) // 4 branch for non-dict attachments."""
        from agentix.bridge.prompt_assembly import trim_context

        cfg = _make_config()
        msg = {
            "role": "user",
            "content": "short",
            "attachments": ["A" * 400],  # string, not dict
        }
        result = trim_context(cfg, [msg], max_tokens=10)
        assert result == []


# ---------------------------------------------------------------------------
# assemble_prompts
# ---------------------------------------------------------------------------


class TestAssemblePrompts:
    def test_adds_system_message(self):
        from agentix.bridge.prompt_assembly import assemble_prompts

        cfg = _make_config(system=["tool_use"])
        with patch("agentix.bridge.prompt_assembly.get_system_prompt", return_value="You are helpful."):
            result = assemble_prompts(cfg, [], max_tokens=1000)
        roles = [m["role"] for m in result.messages]
        assert "system" in roles

    def test_adds_tools_message(self):
        from agentix.bridge.prompt_assembly import assemble_prompts

        cfg = _make_config(tools=["read_file"])
        with patch("agentix.bridge.prompt_assembly.get_tools_prompt", return_value="Tools: read_file"):
            result = assemble_prompts(cfg, [], max_tokens=1000)
        roles = [m["role"] for m in result.messages]
        assert "system" in roles  # tools are added as system messages

    def test_adds_user_message(self):
        from agentix.bridge.prompt_assembly import assemble_prompts

        cfg = _make_config(user=["Hello"])
        result = assemble_prompts(cfg, [], max_tokens=1000)
        roles = [m["role"] for m in result.messages]
        assert "user" in roles

    def test_debug_logging_does_not_crash(self):
        from agentix.bridge.prompt_assembly import assemble_prompts

        cfg = _make_config(system=["tool_use"], user=["Hi"], debug=True)
        with (
            patch("agentix.bridge.prompt_assembly.get_system_prompt", return_value="Sys"),
            patch("agentix.bridge.prompt_assembly.get_user_prompt", return_value="Hi"),
        ):
            result = assemble_prompts(cfg, [], max_tokens=1000)
        assert result is not None

    def test_returns_query_payload_with_model(self):
        from agentix.bridge.prompt_assembly import assemble_prompts

        cfg = _make_config(model="ollama3")
        result = assemble_prompts(cfg, [], max_tokens=500)
        assert result.model == "ollama3"

    def test_file_path_adds_attachment(self):
        """Covers the attachment = get_attachments(args) code path."""
        from agentix.bridge.prompt_assembly import assemble_prompts

        cfg = _make_config(user=["hi"], file_path="/tmp/test.txt")
        mock_attachment = MagicMock()
        mock_attachment.to_dict.return_value = {"content": "file data"}
        with (
            patch("agentix.bridge.prompt_assembly.get_user_prompt", return_value="hi"),
            patch("agentix.bridge.prompt_assembly.get_attachments", return_value=[mock_attachment]),
        ):
            result = assemble_prompts(cfg, [], max_tokens=1000)
        assert result is not None


# ---------------------------------------------------------------------------
# assemble_classification_prompt
# ---------------------------------------------------------------------------


class TestAssembleClassificationPrompt:
    def test_returns_query_payload(self):
        from agentix.context.sessions import assemble_classification_prompt

        cfg = _make_config(user=["Classify me"], model="test-model")
        cfg.classification_max_tokens = 100
        with (
            patch("agentix.bridge.prompt_assembly.get_system_prompt", return_value="Classify sys"),
            patch("agentix.bridge.prompt_assembly.get_user_prompt", return_value="Classify me"),
        ):
            result = assemble_classification_prompt(cfg, [], max_tokens=500)
        assert result is not None

    def test_uses_classification_model_when_provided(self):
        from agentix.context.sessions import assemble_classification_prompt

        cfg = _make_config(user=["x"], model="big-model", classification_model="small-model")
        with (
            patch("agentix.bridge.prompt_assembly.get_system_prompt", return_value="sys"),
            patch("agentix.bridge.prompt_assembly.get_user_prompt", return_value="x"),
        ):
            result = assemble_classification_prompt(cfg, [], max_tokens=500)
        assert result.model == "small-model"


# ---------------------------------------------------------------------------
# manage_sessions
# ---------------------------------------------------------------------------


class TestManageSessions:
    def test_creates_new_session_context(self, tmp_path):
        from agentix.context.sessions import manage_sessions
        from agentix.constants import DEFAULT_SESSION_ID

        cfg = _make_config(session=DEFAULT_SESSION_ID)
        with patch.dict(os.environ, {"AGENTX_SESSIONS_DIR": str(tmp_path), "USER": "alice"}):
            history = manage_sessions(cfg)
        assert isinstance(history, list)

    def test_continue_uses_latest_session(self, tmp_path):
        from agentix.context.sessions import manage_sessions

        user_dir = tmp_path / "alice"
        user_dir.mkdir()
        latest = user_dir / "session_2024-06-01_00-00-00"
        latest.mkdir()
        (latest / "context").mkdir()

        cfg = _make_config(session="__continue")
        with patch.dict(os.environ, {"AGENTX_SESSIONS_DIR": str(tmp_path), "USER": "alice"}):
            with patch("agentix.context.sessions.get_session_history", return_value=[]):
                history = manage_sessions(cfg)
        assert cfg.session == "session_2024-06-01_00-00-00"
        assert isinstance(history, list)

    def test_continue_with_no_sessions_returns_empty(self, tmp_path):
        from agentix.context.sessions import manage_sessions

        cfg = _make_config(session="__continue")
        with patch.dict(os.environ, {"AGENTX_SESSIONS_DIR": str(tmp_path), "USER": "nobody"}):
            history = manage_sessions(cfg)
        assert history == []


# ---------------------------------------------------------------------------
# update_session
# ---------------------------------------------------------------------------


class TestUpdateSession:
    def test_saves_messages_without_file_path(self, tmp_path):
        from agentix.context.sessions import update_session

        cfg = _make_config(session="test-sess")
        mock_msg = MagicMock()
        mock_msg.file_path = None

        with patch.dict(os.environ, {"AGENTX_SESSIONS_DIR": str(tmp_path), "USER": "alice"}):
            with patch("agentix.context.sessions._ensure_session_context_dir", return_value=str(tmp_path)):
                update_session(cfg, [mock_msg], "response")

        mock_msg.save.assert_called_once_with(str(tmp_path))

    def test_skips_messages_with_file_path(self, tmp_path):
        from agentix.context.sessions import update_session

        cfg = _make_config(session="test-sess")
        mock_msg = MagicMock()
        mock_msg.file_path = "/some/file.json"

        with patch.dict(os.environ, {"AGENTX_SESSIONS_DIR": str(tmp_path), "USER": "alice"}):
            with patch("agentix.context.sessions._ensure_session_context_dir", return_value=str(tmp_path)):
                update_session(cfg, [mock_msg], "response")

        mock_msg.save.assert_not_called()


# ---------------------------------------------------------------------------
# get_session_history
# ---------------------------------------------------------------------------


class TestGetSessionHistory:
    def test_returns_list_of_messages(self, tmp_path):
        from agentix.context.sessions import get_session_history

        cfg = _make_config(session="hist-sess")
        with patch.dict(os.environ, {"AGENTX_SESSIONS_DIR": str(tmp_path), "USER": "alice"}):
            result = get_session_history(cfg)
        assert isinstance(result, list)
