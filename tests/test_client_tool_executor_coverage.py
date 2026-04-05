"""Targeted coverage uplift for agentx/integration/client_tool_executor.py.

Covers missing lines:
- _resolve_path security check: path outside base raises ValueError (85-88)
- _resolve_path multi-token path (space-separated paths) (110-111, 119-138)
- _read_file: large file truncation, FileNotFoundError, UnicodeDecodeError, generic error
- _list_directory: not-a-dir, recursive walk, PermissionError, generic error, empty dir
- _write_file: missing content arg, append mode, PermissionError, generic error
- _get_file_info: non-existent path, generic error
- _search_files: missing args, not-a-dir, limit hit, no matches
- _grep_files: all paths including regex error, file-not-found, limit, no matches
"""

import os
import re
import stat
import tempfile
import unittest
from pathlib import Path
from unittest.mock import mock_open, patch, MagicMock

from agentx.integration.client_tool_executor import ClientToolExecutor


def _make_executor(base_path=None) -> ClientToolExecutor:
    return ClientToolExecutor(base_path=base_path)


class TestResolvePathSecurity(unittest.TestCase):
    def test_path_restricted_to_base(self):
        with tempfile.TemporaryDirectory() as tmp:
            executor = _make_executor(base_path=tmp)
            resolved = executor._resolve_path(os.path.join(tmp, "file.txt"))
            self.assertIn(tmp, str(resolved))

    def test_path_outside_base_raises(self):
        with tempfile.TemporaryDirectory() as tmp:
            executor = _make_executor(base_path=tmp)
            with self.assertRaises(ValueError):
                executor._resolve_path("/etc/passwd")

    def test_multitoken_path_uses_last_token(self):
        with tempfile.TemporaryDirectory() as tmp:
            executor = _make_executor(base_path=tmp)
            # Space-separated → last token used (last is inside base_path)
            target = os.path.join(tmp, "file.txt")
            resolved = executor._resolve_path(f"/some/other/path {target}")
            self.assertEqual(str(resolved), str(Path(target).absolute()))

    def test_multitoken_path_raises_when_last_token_outside_base(self):
        with tempfile.TemporaryDirectory() as tmp:
            executor = _make_executor(base_path=tmp)
            with self.assertRaises(ValueError):
                executor._resolve_path("/some/cwd /etc/passwd")

    def test_no_base_path_allows_any_path(self):
        executor = _make_executor(base_path=None)
        resolved = executor._resolve_path("/tmp")
        self.assertIsNotNone(resolved)


class TestReadFileCoverage(unittest.TestCase):
    def test_missing_path_raises(self):
        executor = _make_executor()
        with self.assertRaises(ValueError):
            executor._read_file({})

    def test_large_file_is_truncated(self):
        executor = _make_executor()
        with tempfile.NamedTemporaryFile(mode="w", suffix=".txt", delete=False) as f:
            f.write("x" * 60000)
            tmp_path = f.name
        try:
            result = executor._read_file({"path": tmp_path})
            self.assertIn("truncated", result)
        finally:
            os.unlink(tmp_path)

    def test_file_not_found_returns_error_string(self):
        executor = _make_executor()
        result = executor._read_file({"path": "/nonexistent/path/file.txt"})
        self.assertIn("Error", result)
        self.assertIn("not found", result)

    def test_unicode_decode_error_returns_error_string(self):
        executor = _make_executor()
        with tempfile.NamedTemporaryFile(suffix=".bin", delete=False) as f:
            f.write(b"\xff\xfe\x00\x01")  # Invalid UTF-8
            tmp_path = f.name
        try:
            result = executor._read_file({"path": tmp_path, "encoding": "ascii"})
            self.assertIn("Error", result)
        finally:
            os.unlink(tmp_path)

    def test_generic_exception_returns_error_string(self):
        executor = _make_executor()
        with patch("builtins.open", side_effect=PermissionError("no access")):
            # PermissionError is a subclass of OSError, not FileNotFoundError
            result = executor._read_file({"path": "/some/file.txt"})
            self.assertIn("Error", result)


class TestListDirectoryCoverage(unittest.TestCase):
    def test_missing_path_raises(self):
        executor = _make_executor()
        with self.assertRaises(ValueError):
            executor._list_directory({})

    def test_not_a_directory_returns_error(self):
        executor = _make_executor()
        with tempfile.NamedTemporaryFile(suffix=".txt", delete=False) as f:
            tmp = f.name
        try:
            result = executor._list_directory({"path": tmp})
            self.assertIn("Error", result)
            self.assertIn("Not a directory", result)
        finally:
            os.unlink(tmp)

    def test_empty_directory_returns_empty_message(self):
        executor = _make_executor()
        with tempfile.TemporaryDirectory() as tmp:
            result = executor._list_directory({"path": tmp})
            self.assertIn("Empty directory", result)

    def test_recursive_listing(self):
        executor = _make_executor()
        with tempfile.TemporaryDirectory() as tmp:
            sub = os.path.join(tmp, "subdir")
            os.makedirs(sub)
            with open(os.path.join(sub, "deep.txt"), "w") as f:
                f.write("deep")
            result = executor._list_directory({"path": tmp, "recursive": True})
            self.assertIn("deep.txt", result)

    def test_max_results_truncation(self):
        executor = _make_executor()
        with tempfile.TemporaryDirectory() as tmp:
            for i in range(10):
                with open(os.path.join(tmp, f"file{i}.txt"), "w") as f:
                    f.write("data")
            result = executor._list_directory({"path": tmp, "max_results": 3})
            self.assertIn("truncated", result.lower())

    def test_permission_error_returns_error_string(self):
        executor = _make_executor()
        with patch.object(Path, "is_dir", return_value=True):
            with patch.object(Path, "glob", side_effect=PermissionError("denied")):
                result = executor._list_directory({"path": "/some/dir"})
                self.assertIn("Permission denied", result)


class TestWriteFileCoverage(unittest.TestCase):
    def test_missing_path_raises(self):
        executor = _make_executor()
        with self.assertRaises(ValueError):
            executor._write_file({})

    def test_missing_content_raises(self):
        executor = _make_executor()
        with self.assertRaises(ValueError):
            executor._write_file({"path": "/tmp/test.txt"})

    def test_write_file_creates_parent_dirs(self):
        executor = _make_executor()
        with tempfile.TemporaryDirectory() as tmp:
            nested = os.path.join(tmp, "a", "b", "c.txt")
            result = executor._write_file({"path": nested, "content": "hello"})
            self.assertIn("Wrote to", result)
            self.assertTrue(os.path.exists(nested))

    def test_write_file_append_mode(self):
        executor = _make_executor()
        with tempfile.TemporaryDirectory() as tmp:
            target = os.path.join(tmp, "app.txt")
            executor._write_file({"path": target, "content": "line1\n"})
            result = executor._write_file({"path": target, "content": "line2\n", "append": True})
            self.assertIn("Appended to", result)
            with open(target) as f:
                self.assertEqual(f.read(), "line1\nline2\n")

    def test_write_file_permission_error(self):
        executor = _make_executor()
        with patch("builtins.open", side_effect=PermissionError("no write")):
            with patch.object(Path, "parent", new_callable=lambda: property(lambda s: MagicMock())):
                result = executor._write_file({"path": "/readonly/file.txt", "content": "data"})
                self.assertIn("Permission denied", result)

    def test_write_file_generic_error(self):
        executor = _make_executor()
        with tempfile.TemporaryDirectory() as tmp:
            with patch("builtins.open", side_effect=OSError("disk full")):
                result = executor._write_file({"path": os.path.join(tmp, "f.txt"), "content": "x"})
                self.assertIn("Error writing file", result)


class TestGetFileInfoCoverage(unittest.TestCase):
    def test_missing_path_raises(self):
        executor = _make_executor()
        with self.assertRaises(ValueError):
            executor._get_file_info({})

    def test_nonexistent_path_returns_error(self):
        executor = _make_executor()
        result = executor._get_file_info({"path": "/nonexistent/path/xyz_no_exist.txt"})
        self.assertIn("does not exist", result)

    def test_directory_info(self):
        executor = _make_executor()
        with tempfile.TemporaryDirectory() as tmp:
            import json

            result = executor._get_file_info({"path": tmp})
            info = json.loads(result)
            self.assertTrue(info["is_dir"])

    def test_generic_exception_returns_error_string(self):
        executor = _make_executor()
        with patch.object(Path, "exists", side_effect=OSError("stat failed")):
            result = executor._get_file_info({"path": "/some/file.txt"})
            self.assertIn("Error", result)


class TestSearchFilesCoverage(unittest.TestCase):
    def test_missing_path_raises(self):
        executor = _make_executor()
        with self.assertRaises(ValueError):
            executor._search_files({})

    def test_missing_pattern_raises(self):
        executor = _make_executor()
        with self.assertRaises(ValueError):
            executor._search_files({"path": "/tmp"})

    def test_not_a_directory_returns_error(self):
        executor = _make_executor()
        with tempfile.NamedTemporaryFile(delete=False) as f:
            tmp = f.name
        try:
            result = executor._search_files({"path": tmp, "pattern": "*.py"})
            self.assertIn("Error", result)
        finally:
            os.unlink(tmp)

    def test_no_matching_files_returns_message(self):
        executor = _make_executor()
        with tempfile.TemporaryDirectory() as tmp:
            open(os.path.join(tmp, "file.txt"), "w").close()
            result = executor._search_files({"path": tmp, "pattern": "*.xyz"})
            self.assertIn("No files matching", result)

    def test_limit_hit_returns_truncated_message(self):
        executor = _make_executor()
        with tempfile.TemporaryDirectory() as tmp:
            for i in range(5):
                open(os.path.join(tmp, f"file{i}.py"), "w").close()
            result = executor._search_files({"path": tmp, "pattern": "*.py", "limit": 3})
            self.assertIn("limited to", result)

    def test_recursive_false(self):
        executor = _make_executor()
        with tempfile.TemporaryDirectory() as tmp:
            sub = os.path.join(tmp, "sub")
            os.makedirs(sub)
            open(os.path.join(sub, "deep.py"), "w").close()
            open(os.path.join(tmp, "top.py"), "w").close()
            result = executor._search_files({"path": tmp, "pattern": "*.py", "recursive": False})
            self.assertIn("top.py", result)
            self.assertNotIn("deep.py", result)


class TestGrepFilesCoverage(unittest.TestCase):
    def test_missing_path_raises(self):
        executor = _make_executor()
        with self.assertRaises(ValueError):
            executor._grep_files({})

    def test_missing_pattern_raises(self):
        executor = _make_executor()
        with self.assertRaises(ValueError):
            executor._grep_files({"path": "/tmp"})

    def test_not_a_directory_returns_error(self):
        executor = _make_executor()
        with tempfile.NamedTemporaryFile(suffix=".txt", delete=False) as f:
            tmp = f.name
        try:
            result = executor._grep_files({"path": tmp, "pattern": "hello"})
            self.assertIn("Error", result)
        finally:
            os.unlink(tmp)

    def test_invalid_regex_returns_error(self):
        executor = _make_executor()
        with tempfile.TemporaryDirectory() as tmp:
            result = executor._grep_files({"path": tmp, "pattern": "[invalid(("})
            self.assertIn("Invalid regex", result)

    def test_no_matches_returns_message(self):
        executor = _make_executor()
        with tempfile.TemporaryDirectory() as tmp:
            with open(os.path.join(tmp, "file.py"), "w") as f:
                f.write("print('hello')\n")
            result = executor._grep_files({"path": tmp, "pattern": "zzz_no_match_zzz"})
            self.assertIn("No matches", result)

    def test_finds_matching_lines(self):
        executor = _make_executor()
        with tempfile.TemporaryDirectory() as tmp:
            with open(os.path.join(tmp, "code.py"), "w") as f:
                f.write("# This is a comment\ndef my_function():\n    pass\n")
            result = executor._grep_files({"path": tmp, "pattern": "my_function"})
            self.assertIn("code.py", result)
            self.assertIn("my_function", result)

    def test_case_insensitive_search(self):
        executor = _make_executor()
        with tempfile.TemporaryDirectory() as tmp:
            with open(os.path.join(tmp, "code.py"), "w") as f:
                f.write("MyClass = None\n")
            result = executor._grep_files({"path": tmp, "pattern": "myclass", "ignore_case": True})
            self.assertIn("MyClass", result)

    def test_file_pattern_filter(self):
        executor = _make_executor()
        with tempfile.TemporaryDirectory() as tmp:
            with open(os.path.join(tmp, "code.py"), "w") as f:
                f.write("find_me = True\n")
            with open(os.path.join(tmp, "doc.txt"), "w") as f:
                f.write("find_me is here\n")
            result = executor._grep_files({"path": tmp, "pattern": "find_me", "file_pattern": "*.py"})
            self.assertIn("code.py", result)
            self.assertNotIn("doc.txt", result)

    def test_limit_hit_truncates_results(self):
        executor = _make_executor()
        with tempfile.TemporaryDirectory() as tmp:
            with open(os.path.join(tmp, "code.py"), "w") as f:
                # Write 10 matching lines
                for i in range(10):
                    f.write(f"match_line_{i}\n")
            result = executor._grep_files({"path": tmp, "pattern": "match_line", "limit": 3})
            self.assertIn("limited to", result)

    def test_recursive_search_finds_nested_file(self):
        executor = _make_executor()
        with tempfile.TemporaryDirectory() as tmp:
            sub = os.path.join(tmp, "subpackage")
            os.makedirs(sub)
            with open(os.path.join(sub, "module.py"), "w") as f:
                f.write("def nested_function():\n    pass\n")
            result = executor._grep_files({"path": tmp, "pattern": "nested_function", "recursive": True})
            self.assertIn("nested_function", result)


class TestExecuteDispatch(unittest.TestCase):
    """Test the execute() dispatch method for coverage."""

    def test_execute_unknown_tool_raises(self):
        executor = _make_executor()
        with self.assertRaises(ValueError):
            executor.execute("unknown_tool", {})

    def test_execute_grep_files_dispatches(self):
        executor = _make_executor()
        with tempfile.TemporaryDirectory() as tmp:
            with open(os.path.join(tmp, "f.py"), "w") as f:
                f.write("hello = 1\n")
            result = executor.execute("grep_files", {"path": tmp, "pattern": "hello"})
            self.assertIn("hello", result)

    def test_execute_get_file_info_dispatches(self):
        executor = _make_executor()
        with tempfile.TemporaryDirectory() as tmp:
            f = os.path.join(tmp, "f.txt")
            open(f, "w").close()
            result = executor.execute("get_file_info", {"path": f})
            self.assertIn("is_file", result)

    def test_execute_search_files_dispatches(self):
        executor = _make_executor()
        with tempfile.TemporaryDirectory() as tmp:
            open(os.path.join(tmp, "code.py"), "w").close()
            result = executor.execute("search_files", {"path": tmp, "pattern": "*.py"})
            self.assertIn("code.py", result)


class TestStandaloneWrappers(unittest.TestCase):
    """Test the module-level standalone wrapper functions."""

    def test_standalone_grep_files(self):
        from agentx.integration.client_tool_executor import grep_files

        with tempfile.TemporaryDirectory() as tmp:
            with open(os.path.join(tmp, "code.py"), "w") as f:
                f.write("target_string = 1\n")
            result = grep_files(tmp, "target_string")
            self.assertIn("target_string", result)

    def test_get_client_tool_schemas_returns_list(self):
        from agentx.integration.client_tool_executor import get_client_tool_schemas

        schemas = get_client_tool_schemas()
        self.assertIsInstance(schemas, list)
        self.assertGreater(len(schemas), 0)
        # Each schema should have "type" and "function" keys
        for schema in schemas:
            self.assertIn("type", schema)
            self.assertIn("function", schema)
