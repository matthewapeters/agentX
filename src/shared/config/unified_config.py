"""
Unified configuration for AgentX and Agentix integration.

Configuration is loaded from multiple sources with the following precedence:
1. Default values (lowest)
2. TOML configuration file (agentx.toml)
3. Environment variables
4. CLI arguments (highest)

The configuration is divided into sections:
- AgentXConfig: GUI and client-side settings
- AgentixConfig: Middleware and server-side settings
"""

from dataclasses import dataclass, field
from enum import Enum
from pathlib import Path
from typing import Any, Optional
import os

try:
    import tomllib  # Python 3.11+
except ImportError:
    try:
        import tomli as tomllib  # Fallback for Python < 3.11
    except ImportError:
        tomllib = None  # TOML support disabled


class ScreenSide(str, Enum):
    """Screen position for the AgentX window."""
    LEFT = "left"
    RIGHT = "right"
    CENTER = "center"


@dataclass
class AgentXConfig:
    """
    Configuration for the AgentX GUI client.
    
    These settings control the client-side behavior including
    Ollama connection, GUI layout, and session management.
    """
    
    # Ollama connection (for local/direct mode)
    ollama_host: str = "localhost:11434"
    ollama_model: str = "llama3.2"
    ollama_timeout_seconds: int = 120
    
    # GUI settings
    screen_side: ScreenSide = ScreenSide.LEFT
    window_width: int = 800
    window_height: int = 600
    font_family: str = "Menlo"
    font_size: int = 12
    
    # Session settings
    sessions_dir: str = "sessions"
    auto_save: bool = True
    
    def __post_init__(self):
        """Ensure enums are properly typed."""
        if isinstance(self.screen_side, str):
            self.screen_side = ScreenSide(self.screen_side)
    
    @classmethod
    def from_dict(cls, data: dict) -> "AgentXConfig":
        """Create from dictionary (e.g., from TOML section)."""
        return cls(
            ollama_host=data.get("ollama_host", cls.ollama_host),
            ollama_model=data.get("ollama_model", cls.ollama_model),
            ollama_timeout_seconds=data.get("ollama_initial_load_timeout_seconds", 
                                            data.get("ollama_timeout_seconds", cls.ollama_timeout_seconds)),
            screen_side=data.get("screen_side", cls.screen_side),
            window_width=data.get("window_width", cls.window_width),
            window_height=data.get("window_height", cls.window_height),
            font_family=data.get("font_family", cls.font_family),
            font_size=data.get("font_size", cls.font_size),
            sessions_dir=data.get("sessions_dir", cls.sessions_dir),
            auto_save=data.get("auto_save", cls.auto_save),
        )
    
    def to_dict(self) -> dict:
        """Convert to dictionary."""
        return {
            "ollama_host": self.ollama_host,
            "ollama_model": self.ollama_model,
            "ollama_timeout_seconds": self.ollama_timeout_seconds,
            "screen_side": self.screen_side.value,
            "window_width": self.window_width,
            "window_height": self.window_height,
            "font_family": self.font_family,
            "font_size": self.font_size,
            "sessions_dir": self.sessions_dir,
            "auto_save": self.auto_save,
        }


@dataclass
class AgentixConfig:
    """
    Configuration for the Agentix middleware.
    
    These settings control the middleware behavior including
    classification, tool management, and server connection.
    """
    
    # Integration enablement
    enabled: bool = True
    
    # Server connection (when Agentix is remote)
    server_url: Optional[str] = None  # None means local/embedded
    server_timeout_seconds: int = 300
    
    # Classification settings
    classify_prompts: bool = True
    classification_model: Optional[str] = None  # None means use default model
    show_classification: bool = True
    
    # Tool settings
    available_tools: list[str] = field(default_factory=lambda: ["cst", "ast"])
    show_tool_calls: bool = True
    tool_timeout_seconds: int = 60
    
    # System prompts
    default_system_prompts: list[str] = field(default_factory=list)
    system_prompts_dir: str = "~/.agentix/system_prompts"
    
    # Debug settings
    debug: bool = False
    
    @property
    def is_remote(self) -> bool:
        """Check if Agentix server is remote."""
        return self.server_url is not None
    
    @classmethod
    def from_dict(cls, data: dict) -> "AgentixConfig":
        """Create from dictionary (e.g., from TOML section)."""
        classification_model = data.get("classification_model")
        if classification_model is None:
            classification_model = data.get("agentix_bench_classification_model")
        return cls(
            enabled=data.get("enabled", cls.enabled),
            server_url=data.get("server_url"),
            server_timeout_seconds=data.get("server_timeout_seconds", cls.server_timeout_seconds),
            classify_prompts=data.get("classify_prompts", cls.classify_prompts),
            classification_model=classification_model,
            show_classification=data.get("show_classification", cls.show_classification),
            available_tools=data.get("available_tools", ["cst", "ast"]),
            show_tool_calls=data.get("show_tool_calls", cls.show_tool_calls),
            tool_timeout_seconds=data.get("tool_timeout_seconds", cls.tool_timeout_seconds),
            default_system_prompts=data.get("default_system_prompts", []),
            system_prompts_dir=data.get("system_prompts_dir", cls.system_prompts_dir),
            debug=data.get("debug", cls.debug),
        )
    
    def to_dict(self) -> dict:
        """Convert to dictionary."""
        return {
            "enabled": self.enabled,
            "server_url": self.server_url,
            "server_timeout_seconds": self.server_timeout_seconds,
            "classify_prompts": self.classify_prompts,
            "classification_model": self.classification_model,
            "show_classification": self.show_classification,
            "available_tools": self.available_tools,
            "show_tool_calls": self.show_tool_calls,
            "tool_timeout_seconds": self.tool_timeout_seconds,
            "default_system_prompts": self.default_system_prompts,
            "system_prompts_dir": self.system_prompts_dir,
            "debug": self.debug,
        }


@dataclass
class UnifiedConfig:
    """
    Complete configuration for the integrated AgentX + Agentix system.
    
    Combines both client and middleware configuration into a single
    object that can be loaded from a TOML file.
    
    Example agentx.toml:
        [agentx]
        ollama_host = "localhost:11434"
        ollama_model = "llama3.2"
        screen_side = "left"
        
        [agentix]
        enabled = true
        classify_prompts = true
        available_tools = ["cst", "ast"]
    """
    
    agentx: AgentXConfig = field(default_factory=AgentXConfig)
    agentix: AgentixConfig = field(default_factory=AgentixConfig)
    
    @classmethod
    def from_toml(cls, path: str = "agentx.toml") -> "UnifiedConfig":
        """
        Load configuration from TOML file.
        
        Args:
            path: Path to the TOML configuration file
            
        Returns:
            UnifiedConfig instance
        """
        if tomllib is None:
            raise ImportError(
                "TOML support requires Python 3.11+ or the 'tomli' package. "
                "Install with: uv add tomli"
            )
        
        config_path = Path(path)
        
        if not config_path.exists():
            # Return defaults if file doesn't exist
            return cls()
        
        with open(config_path, "rb") as f:
            data = tomllib.load(f)
        
        return cls.from_dict(data)
    
    @classmethod
    def from_dict(cls, data: dict) -> "UnifiedConfig":
        """
        Create from dictionary.
        
        Args:
            data: Dictionary with 'agentx' and/or 'agentix' sections
            
        Returns:
            UnifiedConfig instance
        """
        agentx_data = data.get("agentx", {})
        agentix_data = data.get("agentix", {})
        
        return cls(
            agentx=AgentXConfig.from_dict(agentx_data),
            agentix=AgentixConfig.from_dict(agentix_data),
        )
    
    def to_dict(self) -> dict:
        """Convert to dictionary (for serialization)."""
        return {
            "agentx": self.agentx.to_dict(),
            "agentix": self.agentix.to_dict(),
        }
    
    @classmethod
    def from_env(cls) -> "UnifiedConfig":
        """
        Create configuration from environment variables.
        
        Environment variables override TOML settings:
        - AGENTX_OLLAMA_HOST
        - AGENTX_OLLAMA_MODEL
        - AGENTIX_SERVER_URL
        - AGENTIX_DEBUG
        etc.
        """
        config = cls()
        
        # AgentX settings
        if host := os.getenv("AGENTX_OLLAMA_HOST"):
            config.agentx.ollama_host = host
        if model := os.getenv("AGENTX_OLLAMA_MODEL"):
            config.agentx.ollama_model = model
        if side := os.getenv("AGENTX_SCREEN_SIDE"):
            config.agentx.screen_side = ScreenSide(side)
        
        # Agentix settings
        if url := os.getenv("AGENTIX_SERVER_URL"):
            config.agentix.server_url = url
        if os.getenv("AGENTIX_ENABLED", "").lower() in ("false", "0", "no"):
            config.agentix.enabled = False
        if os.getenv("AGENTIX_DEBUG", "").lower() in ("true", "1", "yes"):
            config.agentix.debug = True
        
        return config
    
    @classmethod
    def load(cls, toml_path: str = "agentx.toml") -> "UnifiedConfig":
        """
        Load configuration with full precedence chain.
        
        Precedence (highest to lowest):
        1. Environment variables
        2. TOML file
        3. Defaults
        
        Args:
            toml_path: Path to TOML configuration file
            
        Returns:
            UnifiedConfig with all settings merged
        """
        # Start with TOML (includes defaults if file missing)
        config = cls.from_toml(toml_path)
        
        # Override with environment variables
        env_config = cls.from_env()
        
        # Merge environment overrides
        if os.getenv("AGENTX_OLLAMA_HOST"):
            config.agentx.ollama_host = env_config.agentx.ollama_host
        if os.getenv("AGENTX_OLLAMA_MODEL"):
            config.agentx.ollama_model = env_config.agentx.ollama_model
        if os.getenv("AGENTX_SCREEN_SIDE"):
            config.agentx.screen_side = env_config.agentx.screen_side
        if os.getenv("AGENTIX_SERVER_URL"):
            config.agentix.server_url = env_config.agentix.server_url
        if os.getenv("AGENTIX_ENABLED"):
            config.agentix.enabled = env_config.agentix.enabled
        if os.getenv("AGENTIX_DEBUG"):
            config.agentix.debug = env_config.agentix.debug
        
        return config
    
    # Convenience properties for backward compatibility
    
    @property
    def ollama_host(self) -> str:
        """Get Ollama host (AgentX setting)."""
        return self.agentx.ollama_host
    
    @property
    def ollama_model(self) -> str:
        """Get Ollama model (AgentX setting)."""
        return self.agentx.ollama_model
    
    @property
    def agentix_enabled(self) -> bool:
        """Check if Agentix integration is enabled."""
        return self.agentix.enabled
    
    @property
    def is_remote_agentix(self) -> bool:
        """Check if Agentix server is remote."""
        return self.agentix.is_remote


def load_config(path: str = "agentx.toml") -> dict:
    """
    Load configuration as a dictionary.
    
    This function provides backward compatibility with the existing
    AgentX config loading pattern.
    
    Args:
        path: Path to TOML configuration file
        
    Returns:
        Dictionary with configuration sections
    """
    config = UnifiedConfig.load(path)
    return config.to_dict()
