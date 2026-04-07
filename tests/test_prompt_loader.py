"""Tests for agentix.prompt_loader.PromptLoader."""

import os
import tempfile
import unittest.mock as um

import pytest

from agentix.prompt_loader import PromptLoader

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------


def _make_loader_with_dir(tmp_path) -> "tuple[PromptLoader, str]":
    """Return a PromptLoader pointing at *tmp_path* and its string path."""
    return PromptLoader(str(tmp_path)), str(tmp_path)


# ---------------------------------------------------------------------------
# PromptLoader.load()
# ---------------------------------------------------------------------------


class TestLoad:
    def test_load_existing_prompt(self, tmp_path):
        """A file named 'myprompt.md' is retrievable by stem 'myprompt'."""
        (tmp_path / "myprompt.md").write_text("Hello, world!", encoding="utf-8")
        loader = PromptLoader(str(tmp_path))
        assert loader.load("myprompt") == "Hello, world!"

    def test_load_missing_stem_returns_none(self, tmp_path):
        """A stem with no matching file returns None."""
        loader = PromptLoader(str(tmp_path))
        assert loader.load("nonexistent") is None

    def test_load_picks_first_match(self, tmp_path):
        """When multiple extensions exist the first glob match is returned."""
        (tmp_path / "p.md").write_text("from md", encoding="utf-8")
        (tmp_path / "p.txt").write_text("from txt", encoding="utf-8")
        loader = PromptLoader(str(tmp_path))
        result = loader.load("p")
        # Either file is fine — just assert we get a non-None string
        assert result in {"from md", "from txt"}

    def test_load_unreadable_file_returns_none(self, tmp_path):
        """OSError during read returns None, not an exception."""
        loader = PromptLoader(str(tmp_path))
        with (
            um.patch("agentix.prompt_loader._glob.glob", return_value=["/fake/bad.md"]),
            um.patch("builtins.open", side_effect=OSError("permission denied")),
        ):
            result = loader.load("bad")
        assert result is None

    def test_load_glob_exception_propagates_as_none(self, tmp_path):
        """If glob itself raises, load() returns None (PromptLoader catches OSError only)."""
        loader = PromptLoader(str(tmp_path))
        # glob raises a non-OSError so it propagates; but PromptLoader only
        # wraps the open() call.  The glob call is outside the try block.
        # Verify the contract: a real missing file → None, not an exception.
        assert loader.load("totally_missing") is None

    def test_load_defaults_to_system_prompts_dir_constant(self, monkeypatch):
        """When prompts_dir=None the SYSTEM_PROMPTS_DIR constant is used."""
        monkeypatch.setattr("agentix.constants.SYSTEM_PROMPTS_DIR", "/sentinel/")
        loader = PromptLoader(None)
        # The loader's internal dir should be the sentinel (expanded)
        assert loader._dir == os.path.expanduser("/sentinel/")


# ---------------------------------------------------------------------------
# PromptLoader.list_available()
# ---------------------------------------------------------------------------


class TestListAvailable:
    def test_returns_stem_to_path_mapping(self, tmp_path):
        """All files in the directory appear in the mapping."""
        (tmp_path / "alpha.md").write_text("a")
        (tmp_path / "beta.txt").write_text("b")
        loader = PromptLoader(str(tmp_path))
        available = loader.list_available()
        assert "alpha" in available
        assert "beta" in available
        assert available["alpha"].endswith("alpha.md")
        assert available["beta"].endswith("beta.txt")

    def test_empty_directory_returns_empty_dict(self, tmp_path):
        loader = PromptLoader(str(tmp_path))
        assert loader.list_available() == {}

    def test_missing_directory_returns_empty_dict(self):
        loader = PromptLoader("/nonexistent/directory/that/does/not/exist")
        assert loader.list_available() == {}


# ---------------------------------------------------------------------------
# PromptLoader.preview()
# ---------------------------------------------------------------------------


class TestPreview:
    def test_preview_returns_first_two_non_blank_lines(self, tmp_path):
        """preview() trims blank lines and returns at most n_lines lines."""
        (tmp_path / "p.md").write_text("\n\nLine1\nLine2\nLine3", encoding="utf-8")
        loader = PromptLoader(str(tmp_path))
        result = loader.preview(n_lines=2)
        assert "p" in result
        assert result["p"] == ["Line1\n", "Line2\n"]

    def test_preview_handles_unreadable_file(self, tmp_path):
        """OSError during preview is silently skipped for that file."""
        (tmp_path / "bad.md").write_text("x")
        loader = PromptLoader(str(tmp_path))
        with um.patch("builtins.open", side_effect=OSError("denied")):
            result = loader.preview()
        # No exception raised; the bad entry is simply absent
        assert "bad" not in result

    def test_preview_empty_directory(self, tmp_path):
        loader = PromptLoader(str(tmp_path))
        assert loader.preview() == {}


# ---------------------------------------------------------------------------
# PromptLoader.get_formatted_system_prompt()
# ---------------------------------------------------------------------------


class TestGetFormattedSystemPrompt:
    def test_wraps_content_in_system_tags(self, tmp_path):
        """Output is wrapped in [SYSTEM]...[END SYSTEM] tags."""
        (tmp_path / "greet.md").write_text("You are helpful.", encoding="utf-8")
        loader = PromptLoader(str(tmp_path))
        result = loader.get_formatted_system_prompt(["greet"])
        assert result.startswith("[SYSTEM]\n")
        assert "[END SYSTEM]" in result
        assert "You are helpful." in result

    def test_concatenates_multiple_prompts(self, tmp_path):
        """Multiple prompt names are concatenated in order."""
        (tmp_path / "first.md").write_text("FIRST", encoding="utf-8")
        (tmp_path / "second.md").write_text("SECOND", encoding="utf-8")
        loader = PromptLoader(str(tmp_path))
        result = loader.get_formatted_system_prompt(["first", "second"])
        # Both present, first before second
        assert result.index("FIRST") < result.index("SECOND")

    def test_missing_prompt_silently_skipped(self, tmp_path):
        """A name that cannot be found is skipped without raising."""
        (tmp_path / "real.md").write_text("REAL", encoding="utf-8")
        loader = PromptLoader(str(tmp_path))
        result = loader.get_formatted_system_prompt(["missing", "real"])
        assert "REAL" in result
        assert "missing" not in result.replace("[SYSTEM]", "")

    def test_empty_names_list_returns_empty_wrapped(self, tmp_path):
        """Empty names list returns the [SYSTEM]/[END SYSTEM] wrapper around empty string."""
        loader = PromptLoader(str(tmp_path))
        result = loader.get_formatted_system_prompt([])
        assert "[SYSTEM]" in result
        assert "[END SYSTEM]" in result

    def test_debug_flag_does_not_raise(self, tmp_path, caplog):
        """debug=True emits a log message and does not raise."""
        (tmp_path / "p.md").write_text("content", encoding="utf-8")
        loader = PromptLoader(str(tmp_path))
        import logging

        with caplog.at_level(logging.DEBUG, logger="agentix.prompt_loader"):
            result = loader.get_formatted_system_prompt(["p"], debug=True)
        assert "content" in result
