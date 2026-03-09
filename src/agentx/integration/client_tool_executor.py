"""
Client-side tool executor for AgentX.

Implements execution of tools that run on the client machine,
such as file operations and local code analysis.
"""

import os
import json
from pathlib import Path
from typing import Any, Optional, Dict


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
                    raise ValueError(
                        f"Path '{path}' is outside allowed base path '{self.base_path}'"
                    )
        
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
            
        Returns:
            Formatted directory listing
        """
        if "path" not in arguments:
            raise ValueError("list_directory requires 'path' argument")
        
        path = self._resolve_path(arguments["path"])
        recursive = arguments.get("recursive", False)
        pattern = arguments.get("pattern", "*")
        
        try:
            if not path.is_dir():
                return f"Error: Not a directory: {path}"
            
            # Collect files
            files = []
            if recursive:
                items = path.rglob(pattern)
            else:
                items = path.glob(pattern)
            
            for item in sorted(items):
                try:
                    if item.is_dir():
                        files.append(f"[DIR]  {item.relative_to(path)}")
                    else:
                        size = item.stat().st_size
                        files.append(f"[FILE] {item.relative_to(path)} ({size} bytes)")
                except (PermissionError, OSError):
                    files.append(f"[?]    {item.relative_to(path)} (cannot access)")
            
            if not files:
                return f"Empty directory: {path}"
            
            return "\n".join(files)
        
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
            
            # Search for files
            if recursive:
                items = path.rglob(pattern)
            else:
                items = path.glob(pattern)
            
            # Collect results (limited)
            results = []
            for i, item in enumerate(sorted(items)):
                if i >= limit:
                    remaining = sum(1 for _ in path.rglob(pattern)) - limit
                    results.append(f"... and {remaining} more files")
                    break
                results.append(str(item.relative_to(path)))
            
            if not results:
                return f"No files matching pattern '{pattern}' in {path}"
            
            return "\n".join(results)
        
        except Exception as e:
            return f"Error searching files: {str(e)}"


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


def list_directory(path: str, recursive: bool = False, pattern: str = "*") -> str:
    """List the contents of a directory.

    Args:
        path: Absolute or relative path to the directory.
        recursive: If True, list all files in subdirectories as well.
        pattern: Glob pattern to filter results (default: all files).

    Returns:
        A formatted listing with file sizes and directory markers.
    """
    return _get_executor().execute(
        "list_directory",
        {"path": path, "recursive": recursive, "pattern": pattern},
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


# Public names to expose from this module
CLIENT_TOOL_FUNCTIONS = {
    "read_file": read_file,
    "write_file": write_file,
    "list_directory": list_directory,
    "get_file_info": get_file_info,
    "search_files": search_files,
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
    import sys
    from pathlib import Path

    # Ensure agentix is importable (sibling src directory)
    src = str(Path(__file__).resolve().parents[2])
    if src not in sys.path:
        sys.path.insert(0, src)

    from agentix.tools.schema import extract_tool_schema, SchemaGenerationError

    schemas = []
    for fn in CLIENT_TOOL_FUNCTIONS.values():
        try:
            schemas.append(extract_tool_schema(fn))
        except SchemaGenerationError:
            pass  # skip any function that lacks a docstring (shouldn't happen here)
    return schemas

