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
