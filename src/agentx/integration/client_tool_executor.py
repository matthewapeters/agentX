"""
Client-side tool executor for AgentX.

Implements execution of tools that run on the client machine,
such as file operations and local code analysis.
"""

import os
import json
import re
from pathlib import Path
from typing import Any, Optional, Dict

# Directories to skip when doing recursive traversal (common noise sources)
_EXCLUDE_DIRS = frozenset(
    {
        ".venv",
        "venv",
        ".env",
        "__pycache__",
        ".git",
        ".hg",
        ".svn",
        "node_modules",
        ".tox",
        ".mypy_cache",
        ".pytest_cache",
        ".ruff_cache",
        "dist",
        "build",
        "*.egg-info",
        ".eggs",
        "htmlcov",
        ".coverage",
    }
)


class ClientToolExecutor:
    """
    Executes tools on the client side.

    Supported tools:
    - read_file: Read file contents
    - list_directory: List directory contents
    - write_file: Write to file
    - get_file_info: Get file metadata
    - search_files: Search for files matching pattern
    """

    def __init__(self, base_path: Optional[str] = None):
        """
        Initialize executor with optional base path for security.

        Args:
            base_path: If set, all file operations restricted to this path
        """
        self.base_path = Path(base_path) if base_path else None

    def execute(self, tool_name: str, arguments: dict) -> str:
        """
        Execute a client-side tool.

        Args:
            tool_name: Name of the tool to execute
            arguments: Tool arguments

        Returns:
            Tool execution result as string

        Raises:
            ValueError: If tool not found or arguments invalid
        """
        match tool_name:
            case "read_file":
                return self._read_file(arguments)
            case "list_directory":
                return self._list_directory(arguments)
            case "write_file":
                return self._write_file(arguments)
            case "get_file_info":
                return self._get_file_info(arguments)
            case "search_files":
                return self._search_files(arguments)
            case "grep_files":
                return self._grep_files(arguments)
            case _:
                raise ValueError(f"Unknown client-side tool: {tool_name}")

    def _resolve_path(self, path: str) -> Path:
        """
        Resolve a file path with security checks.

        Args:
            path: Path to resolve

        Returns:
            Resolved Path object

        Raises:
            ValueError: If path is outside base_path
        """
        # Sanitize: strip surrounding whitespace and take the first token if the
        # LLM accidentally passes a space-separated multi-path string such as
        # "/Projects/agentX/ /Projects/project_001".
        path = path.strip()
        if " " in path:
            # Use the last token — the LLM tends to prepend the process cwd then
            # the intended path, so the rightmost part is the intended target.
            tokens = [t for t in path.split() if t]
            path = tokens[-1]

        file_path = Path(path).expanduser()

        # Resolve both paths to handle symlinks properly
        # Use resolve() only on the base_path if it exists
        if self.base_path:
            # Resolve base_path accounting for macOS /var symlink issue
            try:
                base_resolved = self.base_path.resolve()
            except:
                base_resolved = self.base_path

            # For file_path, resolve() might fail if file doesn't exist yet
            try:
                file_resolved = file_path.resolve()
            except:
                file_resolved = file_path

            # Security check: ensure file path is within base path
            try:
                file_resolved.relative_to(base_resolved)
            except ValueError:
                # Also try without resolving symlinks as backup
                try:
                    Path(file_path).absolute().relative_to(Path(self.base_path).absolute())
                except ValueError:
                    raise ValueError(f"Path '{path}' is outside allowed base path '{self.base_path}'")

        # Return the unresolved path if possible (preserves relative paths)
        return file_path.absolute()

    def _read_file(self, arguments: dict) -> str:
        """
        Read file contents.

        Arguments:
            path: Path to file (required)
            encoding: File encoding (default: utf-8)

        Returns:
            File contents
        """
        if "path" not in arguments:
            raise ValueError("read_file requires 'path' argument")

        path = self._resolve_path(arguments["path"])
        encoding = arguments.get("encoding", "utf-8")

        try:
            with open(path, "r", encoding=encoding) as f:
                contents = f.read()

            # Limit output size to prevent overwhelming responses
            max_size = 50000  # 50KB
            if len(contents) > max_size:
                return f"{contents[:max_size]}\n\n[... file truncated, {len(contents) - max_size} bytes omitted ...]"

            return contents

        except FileNotFoundError:
            return f"Error: File not found: {path}"
        except UnicodeDecodeError:
            return f"Error: Could not decode file as {encoding}: {path}"
        except Exception as e:
            return f"Error reading file: {str(e)}"

    def _list_directory(self, arguments: dict) -> str:
        """
        List directory contents.

        Arguments:
            path: Path to directory (required)
            recursive: List recursively (default: false)
            pattern: File pattern to match (default: *)
            max_results: Maximum number of entries to return (default: 500)
            max_depth: Maximum recursion depth when recursive=True (default: 3)

        Returns:
            Formatted directory listing
        """
        if "path" not in arguments:
            raise ValueError("list_directory requires 'path' argument")

        path = self._resolve_path(arguments["path"])
        recursive = arguments.get("recursive", False)
        pattern = arguments.get("pattern", "*")
        max_results = int(arguments.get("max_results", 500))
        max_depth = int(arguments.get("max_depth", 3))

        try:
            if not path.is_dir():
                return f"Error: Not a directory: {path}"

            files: list[str] = []
            truncated = False

            if not recursive:
                items = sorted(path.glob(pattern))
                for item in items:
                    if len(files) >= max_results:
                        truncated = True
                        break
                    try:
                        if item.is_dir():
                            files.append(f"[DIR]  {item.relative_to(path)}/")
                        else:
                            size = item.stat().st_size
                            files.append(f"[FILE] {item.relative_to(path)} ({size} bytes)")
                    except (PermissionError, OSError):
                        files.append(f"[?]    {item.relative_to(path)} (cannot access)")
            else:
                # Depth-limited, filtered recursive walk
                def _walk(directory: Path, depth: int) -> None:
                    nonlocal truncated
                    if depth > max_depth or truncated:
                        return
                    try:
                        entries = sorted(directory.iterdir())
                    except PermissionError:
                        return
                    for entry in entries:
                        if truncated or len(files) >= max_results:
                            truncated = True
                            return
                        # Skip excluded directories
                        if entry.is_dir() and entry.name in _EXCLUDE_DIRS:
                            continue
                        # Apply pattern filter to files (dirs always shown at their level)
                        try:
                            rel = entry.relative_to(path)
                            if entry.is_dir():
                                files.append(f"[DIR]  {rel}/")
                                _walk(entry, depth + 1)
                            elif entry.match(pattern):
                                size = entry.stat().st_size
                                files.append(f"[FILE] {rel} ({size} bytes)")
                        except (PermissionError, OSError):
                            files.append(f"[?]    {entry.relative_to(path)} (cannot access)")

                _walk(path, 1)

            if not files:
                return f"Empty directory: {path}"

            result = "\n".join(files)
            if truncated:
                result += f"\n\n[Listing truncated at {max_results} entries. Use a subdirectory path or pattern to narrow results.]"
            return result

        except PermissionError:
            return f"Error: Permission denied: {path}"
        except Exception as e:
            return f"Error listing directory: {str(e)}"

    def _write_file(self, arguments: dict) -> str:
        """
        Write to file.

        Arguments:
            path: Path to file (required)
            content: Content to write (required)
            append: Append instead of overwrite (default: false)
            encoding: File encoding (default: utf-8)

        Returns:
            Success/error message
        """
        if "path" not in arguments:
            raise ValueError("write_file requires 'path' argument")
        if "content" not in arguments:
            raise ValueError("write_file requires 'content' argument")

        path = self._resolve_path(arguments["path"])
        content = arguments["content"]
        append = arguments.get("append", False)
        encoding = arguments.get("encoding", "utf-8")

        try:
            # Create parent directory if needed
            path.parent.mkdir(parents=True, exist_ok=True)

            mode = "a" if append else "w"
            with open(path, mode, encoding=encoding) as f:
                f.write(content)

            action = "Appended to" if append else "Wrote to"
            return f"{action} file: {path}"

        except PermissionError:
            return f"Error: Permission denied: {path}"
        except Exception as e:
            return f"Error writing file: {str(e)}"

    def _get_file_info(self, arguments: dict) -> str:
        """
        Get file metadata.

        Arguments:
            path: Path to file (required)

        Returns:
            JSON-formatted file information
        """
        if "path" not in arguments:
            raise ValueError("get_file_info requires 'path' argument")

        path = self._resolve_path(arguments["path"])

        try:
            if not path.exists():
                return f"Error: Path does not exist: {path}"

            stat = path.stat()
            info = {
                "path": str(path),
                "exists": True,
                "is_file": path.is_file(),
                "is_dir": path.is_dir(),
                "is_symlink": path.is_symlink(),
                "size_bytes": stat.st_size,
                "modified": stat.st_mtime,
                "created": stat.st_ctime,
                "permissions": oct(stat.st_mode)[-3:],
            }

            return json.dumps(info, indent=2)

        except Exception as e:
            return f"Error getting file info: {str(e)}"

    def _search_files(self, arguments: dict) -> str:
        """
        Search for files matching pattern.

        Arguments:
            path: Search root directory (required)
            pattern: File pattern (required, e.g., "*.py")
            recursive: Search recursively (default: true)
            limit: Maximum results (default: 100)

        Returns:
            List of matching file paths
        """
        if "path" not in arguments:
            raise ValueError("search_files requires 'path' argument")
        if "pattern" not in arguments:
            raise ValueError("search_files requires 'pattern' argument")

        path = self._resolve_path(arguments["path"])
        pattern = arguments["pattern"]
        recursive = arguments.get("recursive", True)
        limit = arguments.get("limit", 100)

        try:
            if not path.is_dir():
                return f"Error: Not a directory: {path}"

            results: list[str] = []
            count = 0

            def _collect(directory: Path) -> None:
                nonlocal count
                try:
                    entries = sorted(directory.iterdir())
                except PermissionError:
                    return
                for entry in entries:
                    if count >= limit:
                        return
                    if entry.is_dir():
                        if entry.name in _EXCLUDE_DIRS:
                            continue
                        if recursive:
                            _collect(entry)
                    elif entry.match(pattern):
                        results.append(str(entry.relative_to(path)))
                        count += 1

            _collect(path)

            if not results:
                return f"No files matching pattern '{pattern}' in {path}"

            output = "\n".join(results)
            if count >= limit:
                output += f"\n\n[Results limited to {limit}. Use a more specific pattern or subdirectory.]"
            return output

        except Exception as e:
            return f"Error searching files: {str(e)}"

    def _grep_files(self, arguments: dict) -> str:
        """
        Search file contents for a regex or string pattern.

        Arguments:
            path: Root directory to search in (required)
            pattern: String or regex pattern to search for in file contents (required)
            file_pattern: Glob to restrict which files are searched (default: "*.py")
            recursive: Search subdirectories (default: true)
            ignore_case: Case-insensitive matching (default: false)
            limit: Maximum number of matching lines to return (default: 200)

        Returns:
            Matching lines with file:line_number context.
        """
        if "path" not in arguments:
            raise ValueError("grep_files requires 'path' argument")
        if "pattern" not in arguments:
            raise ValueError("grep_files requires 'pattern' argument")

        root = self._resolve_path(arguments["path"])
        pattern = arguments["pattern"]
        file_pattern = arguments.get("file_pattern", "*.py")
        recursive = arguments.get("recursive", True)
        ignore_case = arguments.get("ignore_case", False)
        limit = int(arguments.get("limit", 200))

        try:
            if not root.is_dir():
                return f"Error: Not a directory: {root}"

            flags = re.IGNORECASE if ignore_case else 0
            try:
                regex = re.compile(pattern, flags)
            except re.error as e:
                return f"Error: Invalid regex pattern '{pattern}': {e}"

            matches: list[str] = []
            total = 0

            def _search_file(file_path: Path) -> None:
                nonlocal total
                try:
                    with open(file_path, "r", encoding="utf-8", errors="replace") as f:
                        for lineno, line in enumerate(f, 1):
                            if total >= limit:
                                return
                            if regex.search(line):
                                rel = file_path.relative_to(root)
                                matches.append(f"{rel}:{lineno}: {line.rstrip()}")
                                total += 1
                except (PermissionError, OSError):
                    pass

            def _walk(directory: Path) -> None:
                try:
                    entries = sorted(directory.iterdir())
                except PermissionError:
                    return
                for entry in entries:
                    if total >= limit:
                        return
                    if entry.is_dir():
                        if entry.name in _EXCLUDE_DIRS:
                            continue
                        if recursive:
                            _walk(entry)
                    elif entry.match(file_pattern):
                        _search_file(entry)

            _walk(root)

            if not matches:
                return f"No matches for '{pattern}' in {root} (file filter: {file_pattern})"

            output = "\n".join(matches)
            if total >= limit:
                output += f"\n\n[Results limited to {limit} lines. Narrow your search or increase limit.]"
            return output

        except Exception as e:
            return f"Error searching file contents: {str(e)}"


# ---------------------------------------------------------------------------
# Standalone tool functions
#
# These thin wrappers have proper type signatures and docstrings so that
# ``extract_tool_schema()`` can auto-generate OpenAI JSON schemas for them.
# They delegate to a module-level ClientToolExecutor instance.
# ---------------------------------------------------------------------------

_executor: "ClientToolExecutor | None" = None


def _get_executor() -> "ClientToolExecutor":
    global _executor
    if _executor is None:
        _executor = ClientToolExecutor()
    return _executor


def read_file(path: str, encoding: str = "utf-8") -> str:
    """Read the contents of a file from the filesystem.

    Args:
        path: Absolute or relative path to the file.
        encoding: Text encoding to use when reading (default: utf-8).

    Returns:
        The file contents as a string (truncated to 50 KB if larger).
    """
    return _get_executor().execute("read_file", {"path": path, "encoding": encoding})


def write_file(path: str, content: str, append: bool = False, encoding: str = "utf-8") -> str:
    """Write text content to a file on the filesystem.

    Creates the file and any missing parent directories automatically.

    Args:
        path: Absolute or relative path to the file to write.
        content: The text content to write.
        append: If True, append to the file instead of overwriting it.
        encoding: Text encoding to use when writing (default: utf-8).

    Returns:
        A success message with the resolved file path.
    """
    return _get_executor().execute(
        "write_file",
        {"path": path, "content": content, "append": append, "encoding": encoding},
    )


def list_directory(
    path: str,
    recursive: bool = False,
    pattern: str = "*",
    max_results: int = 500,
    max_depth: int = 3,
) -> str:
    """List the contents of a directory.

    Directories named ``.venv``, ``__pycache__``, ``.git``, ``node_modules``,
    and other common noise sources are automatically skipped during recursive
    traversal to avoid overwhelming output.

    Args:
        path: Absolute or relative path to the directory.
        recursive: If True, list all files in subdirectories as well.
        pattern: Glob pattern to filter results (default: all files).
        max_results: Maximum number of entries to return (default: 500).
        max_depth: Maximum directory depth when ``recursive`` is True (default: 3).

    Returns:
        A formatted listing with file sizes and directory markers.
    """
    return _get_executor().execute(
        "list_directory",
        {
            "path": path,
            "recursive": recursive,
            "pattern": pattern,
            "max_results": max_results,
            "max_depth": max_depth,
        },
    )


def get_file_info(path: str) -> str:
    """Get metadata for a file or directory.

    Args:
        path: Absolute or relative path to inspect.

    Returns:
        JSON-formatted object with size, timestamps, permissions, and type flags.
    """
    return _get_executor().execute("get_file_info", {"path": path})


def search_files(path: str, pattern: str, recursive: bool = True, limit: int = 100) -> str:
    """Search for files matching a glob pattern within a directory.

    Args:
        path: Root directory to search in.
        pattern: Glob pattern to match filenames (e.g. ``*.py``, ``test_*.txt``).
        recursive: If True, search all subdirectories (default: True).
        limit: Maximum number of results to return (default: 100).

    Returns:
        Newline-separated list of matching relative file paths.
    """
    return _get_executor().execute(
        "search_files",
        {"path": path, "pattern": pattern, "recursive": recursive, "limit": limit},
    )


def grep_files(
    path: str,
    pattern: str,
    file_pattern: str = "*.py",
    recursive: bool = True,
    ignore_case: bool = False,
    limit: int = 200,
) -> str:
    """Search file contents for a regex or plain-text pattern.

    Unlike ``search_files`` (which matches file *names*), this tool searches
    *inside* files and returns matching lines with file:line context, similar
    to ``grep -rn``.

    Args:
        path: Root directory to search in.
        pattern: String or regex pattern to search for in file contents.
        file_pattern: Glob pattern restricting which files are searched (default: ``*.py``).
        recursive: If True, search all subdirectories (default: True).
        ignore_case: If True, perform case-insensitive matching (default: False).
        limit: Maximum number of matching lines to return (default: 200).

    Returns:
        Matching lines formatted as ``file:line_number: content``.
    """
    return _get_executor().execute(
        "grep_files",
        {
            "path": path,
            "pattern": pattern,
            "file_pattern": file_pattern,
            "recursive": recursive,
            "ignore_case": ignore_case,
            "limit": limit,
        },
    )


# Public names to expose from this module
CLIENT_TOOL_FUNCTIONS = {
    "read_file": read_file,
    "write_file": write_file,
    "list_directory": list_directory,
    "get_file_info": get_file_info,
    "search_files": search_files,
    "grep_files": grep_files,
}


def get_client_tool_implementations() -> dict:
    """Return a mapping of client tool name → callable.

    All returned functions are safe to call with keyword arguments extracted
    from an LLM tool-call response.
    """
    return dict(CLIENT_TOOL_FUNCTIONS)


def get_client_tool_schemas() -> list:
    """Return OpenAI-format tool schemas for all client tools.

    Uses ``extract_tool_schema()`` so schemas always stay in sync with the
    function signatures and docstrings above.

    Returns:
        List of dicts in the ``{"type": "function", "function": {...}}`` format.
    """
    from agentix.tools.schema import extract_tool_schema, SchemaGenerationError

    schemas = []
    for fn in CLIENT_TOOL_FUNCTIONS.values():
        try:
            schemas.append(extract_tool_schema(fn))
        except SchemaGenerationError:
            pass  # skip any function that lacks a docstring (shouldn't happen here)
    return schemas
