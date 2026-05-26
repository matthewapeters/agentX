"""Tool registry for managing available tools in AgentX.

Provides dynamic loading of tools from agentx_tools.toml, enable/disable
toggling, and built-in tools for tool management.

Tool schema supports both legacy and extended metadata:
- Legacy: description, category, enabled
- Extended: scope, source_path, runtime, entrypoint, input_schema, output_schema
"""

import json
from pathlib import Path
from typing import Any, Optional

import toml

from src.agentx.config import DEFAULT_CONFIG, DEFAULT_TOOL_REGISTRY_SEARCH_PATHS

DEFAULT_AGENTX_TOOLS_TOML = """# AgentX Dynamic Tool Registry
# Define available tools that the agent can invoke.
# Each tool must have a unique name.
#
# Extended tool schema fields:
# - description: human-readable tool description
# - category: logical category for UI grouping
# - enabled: whether the tool is currently available
# - scope: universal | user | project | session
# - source_path: path to implementation script/module
# - runtime: execution runtime (python, shell, etc)
# - entrypoint: callable reference (module:function or function name)
# - input_schema: JSON-schema-like object for parameters
# - output_schema: JSON-schema-like object for return shape

schema_version = "1.1"

[tool_paths]
universal = "~/.agentx/tools/universal"
user = "~/.agentx/tools/user"
project = "./tools"
session = "./sessions/_tools"

[tools.cst]
description = "Concrete Syntax Tree analysis - parse and analyze code structure"
category = "code-analysis"
enabled = true

[tools.ast]
description = "Abstract Syntax Tree analysis - analyze code semantics"
category = "code-analysis"
enabled = true

[tools.read_file]
description = "Read file contents"
category = "filesystem"
enabled = true

[tools.write_file]
description = "Write or create files"
category = "filesystem"
enabled = true

[tools.list_directory]
description = "List directory contents"
category = "filesystem"
enabled = true

[tools.get_file_info]
description = "Get file metadata (size, permissions, timestamps)"
category = "filesystem"
enabled = true

[tools.search_files]
description = "Search for files matching patterns"
category = "filesystem"
enabled = true

# Example external Python tool registration:
# [tools.extract_todos]
# description = "Extract TODO comments from source files"
# category = "analysis"
# enabled = true
# scope = "project"
# source_path = "tools/extract_todos.py"
# runtime = "python"
# entrypoint = "main"
# [tools.extract_todos.input_schema]
# type = "object"
# required = ["path"]
# [tools.extract_todos.input_schema.properties.path]
# type = "string"
# description = "Root path to scan"
# [tools.extract_todos.output_schema]
# type = "object"
# required = ["todos"]
# [tools.extract_todos.output_schema.properties.todos]
# type = "array"

# Built-in system tools (always available, not overrideable)
[system_tools]
reload_tools = true
register_tool = true
diagnose_tools = true
open_file_in_editor = true
diff_files_in_editor = true
editor_action = true
"""


class ToolRegistry:
    """
    Manages available tools for AgentX.

    Loads tool definitions from agentx_tools.toml, maintains enabled/disabled state,
    and provides built-in tools for tool management.

    Example:
        registry = ToolRegistry(config_path="agentx_tools.toml")
        all_tools = registry.get_all_tools()
        enabled = registry.get_enabled_tools()
        registry.toggle_tool("cst", enabled=False)
        registry.register_tool("my_tool", {"description": "Custom tool"})
        registry.reload_tools()
    """

    def __init__(self, config_path: Optional[str] = None):
        """
        Initialize tool registry.

        Args:
            config_path: Path to agentx_tools.toml. If None, uses default location.
        """
        self.config_path = self._resolve_config_path(config_path)
        self.tools: dict[str, dict[str, Any]] = {}
        self.enabled_state: dict[str, bool] = {}
        self._load_from_config()

    @classmethod
    def _read_search_paths_from_agentx_config(cls) -> list[str]:
        """Read configured tool-registry search paths from agentx.toml.

        Returns:
            Ordered list of configured search paths, or defaults if missing.
        """
        config_file = Path(DEFAULT_CONFIG)
        if not config_file.exists():
            return list(DEFAULT_TOOL_REGISTRY_SEARCH_PATHS)

        try:
            config = toml.load(config_file)
        except toml.TomlDecodeError:
            return list(DEFAULT_TOOL_REGISTRY_SEARCH_PATHS)

        tool_registry = config.get("tool_registry", {})
        search_paths = tool_registry.get("search_paths", [])
        if isinstance(search_paths, list):
            normalized = [p for p in search_paths if isinstance(p, str) and p.strip()]
            if normalized:
                return normalized
        return list(DEFAULT_TOOL_REGISTRY_SEARCH_PATHS)

    @classmethod
    def _materialize_default_config(cls, path: Path) -> None:
        """Create a default tool registry TOML file at the given path."""
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(DEFAULT_AGENTX_TOOLS_TOML, encoding="utf-8")

    @classmethod
    def _resolve_config_path(cls, config_path: Optional[str]) -> Path:
        """Resolve the registry config path and auto-generate missing default file.

        Resolution order:
        1. Explicit ``config_path`` when provided.
        2. Search paths from ``agentx.toml`` under ``[tool_registry].search_paths``.
        3. Built-in defaults when the config is missing/invalid.

        If no candidate exists on disk, the first candidate is created from the
        baked-in default template.
        """
        if config_path:
            candidate = Path(config_path).expanduser()
            if not candidate.exists():
                cls._materialize_default_config(candidate)
            return candidate

        candidates = []
        for raw_path in cls._read_search_paths_from_agentx_config():
            candidate = Path(raw_path).expanduser()
            if not candidate.is_absolute():
                candidate = Path.cwd() / candidate
            candidates.append(candidate)

        for candidate in candidates:
            if candidate.exists():
                return candidate

        target = candidates[0] if candidates else (Path.cwd() / "agentx_tools.toml")
        cls._materialize_default_config(target)
        return target

    @staticmethod
    def _normalize_schema(value: Any, fallback_description: str) -> dict[str, Any]:
        """Normalize input/output schema values into JSON-schema-like dictionaries.

        Args:
            value: Schema value from TOML, can be table, JSON string, or None.
            fallback_description: Description to use when a default schema is generated.

        Returns:
            Normalized schema dictionary.
        """
        if isinstance(value, dict):
            return value

        if isinstance(value, str) and value.strip():
            try:
                parsed = json.loads(value)
                if isinstance(parsed, dict):
                    return parsed
            except json.JSONDecodeError:
                pass

        return {
            "type": "object",
            "properties": {},
            "required": [],
            "description": fallback_description,
        }

    def _load_from_config(self) -> None:
        """Load tool definitions from agentx_tools.toml.

        Raises:
            FileNotFoundError: If config file doesn't exist.
            ValueError: If config format is invalid.
        """
        if not self.config_path.exists():
            raise FileNotFoundError(f"Tool registry config not found: {self.config_path}")

        try:
            config = toml.load(self.config_path)
        except toml.TomlDecodeError as e:
            raise ValueError(f"Invalid TOML in {self.config_path}: {e}") from e

        tools_section = config.get("tools", {})
        if not isinstance(tools_section, dict):
            raise ValueError("[tools] section must be a table")

        self.tools.clear()
        self.enabled_state.clear()

        for tool_name, tool_def in tools_section.items():
            if not isinstance(tool_def, dict):
                raise ValueError(f"Tool {tool_name} must be a table")

            self.tools[tool_name] = {
                "name": tool_name,
                "description": tool_def.get("description", ""),
                "category": tool_def.get("category", "user"),
                "scope": tool_def.get("scope", "project"),
                "source_path": tool_def.get("source_path"),
                "runtime": tool_def.get("runtime", "python"),
                "entrypoint": tool_def.get("entrypoint"),
                "input_schema": self._normalize_schema(
                    tool_def.get("input_schema"),
                    f"Input schema for tool '{tool_name}'",
                ),
                "output_schema": self._normalize_schema(
                    tool_def.get("output_schema"),
                    f"Output schema for tool '{tool_name}'",
                ),
            }
            self.enabled_state[tool_name] = bool(tool_def.get("enabled", True))

    def get_all_tools(self) -> list[dict[str, Any]]:
        """Get all tools (enabled and disabled).

        Returns:
            List of tool definitions with name, description, category, and enabled status.
        """
        return [{**tool, "enabled": self.enabled_state.get(name, True)} for name, tool in self.tools.items()]

    def get_enabled_tools(self) -> list[dict[str, Any]]:
        """Get only enabled tools.

        Returns:
            List of enabled tool definitions.
        """
        return [{**tool, "enabled": True} for name, tool in self.tools.items() if self.enabled_state.get(name, True)]

    def get_enabled_tool_names(self) -> list[str]:
        """Get list of enabled tool names.

        Returns:
            List of tool names that are currently enabled.
        """
        return [name for name, enabled in self.enabled_state.items() if enabled]

    def toggle_tool(self, tool_name: str, enabled: bool) -> bool:
        """Enable or disable a tool.

        Args:
            tool_name: Name of the tool to toggle.
            enabled: Whether to enable (True) or disable (False).

        Returns:
            True if the tool state was changed, False if tool doesn't exist.
        """
        if tool_name not in self.tools:
            return False
        self.enabled_state[tool_name] = enabled
        return True

    def register_tool(
        self,
        tool_name: str,
        description: str = "",
        category: str = "user",
        enabled: bool = True,
        scope: str = "project",
        source_path: Optional[str] = None,
        runtime: str = "python",
        entrypoint: Optional[str] = None,
        input_schema: Optional[dict[str, Any]] = None,
        output_schema: Optional[dict[str, Any]] = None,
        persist: bool = False,
    ) -> bool:
        """Register a new tool dynamically.

        Args:
            tool_name: Unique name for the tool.
            description: Human-readable description.
            category: Tool category (e.g., "user", "filesystem", "code-analysis").
            enabled: Whether the tool is enabled by default.
            scope: Tool scope (e.g., "universal", "user", "project", "session").
            source_path: Optional path to source file implementing this tool.
            runtime: Tool runtime identifier (e.g., "python").
            entrypoint: Optional callable/function entrypoint (e.g., "module:function").
            input_schema: Optional JSON-schema-like input definition.
            output_schema: Optional JSON-schema-like output definition.
            persist: Whether to persist the new tool entry to the TOML config.

        Returns:
            True if the tool was registered, False if it already exists.
        """
        if tool_name in self.tools:
            return False

        normalized_input_schema = self._normalize_schema(input_schema, f"Input schema for tool '{tool_name}'")
        normalized_output_schema = self._normalize_schema(output_schema, f"Output schema for tool '{tool_name}'")

        self.tools[tool_name] = {
            "name": tool_name,
            "description": description,
            "category": category,
            "scope": scope,
            "source_path": source_path,
            "runtime": runtime,
            "entrypoint": entrypoint,
            "input_schema": normalized_input_schema,
            "output_schema": normalized_output_schema,
        }
        self.enabled_state[tool_name] = enabled

        if persist:
            self._persist_tool(tool_name)

        return True

    def _persist_tool(self, tool_name: str) -> None:
        """Persist a single tool definition to the TOML config.

        Args:
            tool_name: Name of tool to persist.

        Raises:
            FileNotFoundError: If the config file does not exist.
            ValueError: If the config file contains invalid TOML.
        """
        if tool_name not in self.tools:
            return

        if not self.config_path.exists():
            raise FileNotFoundError(f"Tool registry config not found: {self.config_path}")

        try:
            config = toml.load(self.config_path)
        except toml.TomlDecodeError as e:
            raise ValueError(f"Invalid TOML in {self.config_path}: {e}") from e

        if "tools" not in config or not isinstance(config["tools"], dict):
            config["tools"] = {}

        tool = self.tools[tool_name]
        config["tools"][tool_name] = {
            "description": tool.get("description", ""),
            "category": tool.get("category", "user"),
            "enabled": bool(self.enabled_state.get(tool_name, True)),
            "scope": tool.get("scope", "project"),
            "source_path": tool.get("source_path"),
            "runtime": tool.get("runtime", "python"),
            "entrypoint": tool.get("entrypoint"),
            "input_schema": tool.get("input_schema", {}),
            "output_schema": tool.get("output_schema", {}),
        }

        with self.config_path.open("w", encoding="utf-8") as file_handle:
            toml.dump(config, file_handle)

    def reload_tools(self) -> None:
        """Reload tool definitions from config file.

        Resets enabled/disabled state to config defaults as stored in TOML.
        Dynamically registered tools are retained only if they were persisted.

        Raises:
            FileNotFoundError: If config file doesn't exist.
            ValueError: If config format is invalid.
        """
        self._load_from_config()

    def save_enabled_state(self, output_path: Optional[Path] = None) -> None:
        """Save current enabled/disabled state to a file.

        Useful for persisting user preferences across sessions.

        Args:
            output_path: Path to save state to. If None, uses registry config path.
        """
        output = output_path or self.config_path
        state = {"tools": {name: {"enabled": self.enabled_state.get(name, True)} for name in self.tools}}
        with open(output, "w") as f:
            toml.dump(state, f)

    def to_dict(self) -> dict[str, Any]:
        """Export registry state as a dictionary.

        Returns:
            Dictionary containing all tools and their enabled state.
        """
        return {
            "tools": self.get_all_tools(),
            "enabled_tools": self.get_enabled_tool_names(),
        }
