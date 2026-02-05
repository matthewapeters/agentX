"""
Docstring for agentix
"""

from . import agentix_config, api_client, main
from .agent import agentix
from .agentix_config import AgentixConfig

# from .api_client import query_api, summarize_user_prompt
from .constants import (
    DEFAULT_SESSION_ID,
    DEFAULT_TEMPERATURE,
    MAX_TOKENS,
    OLLAMA_API_BASE,
    SESSIONS_DIR,
    SESSIONS_METADATA_FILE,
    SYSTEM_PROMPTS_DIR,
)
from .context import Message
from .context.prompts import get_prompts, get_system_prompt, get_user_prompt
from .context.sessions import (
    assemble_prompts,
    get_session_history,
    manage_sessions,
    trim_context,
)
from .file_utils import get_attachments, get_file, load_file
from .main import main as __main__
from .models import get_model, get_models
from .transforms import transform_ollama_tags_to_openai_engines

__all__ = [
    "AgentixConfig",
    "DEFAULT_SESSION_ID",
    "DEFAULT_TEMPERATURE",
    "MAX_TOKENS",
    "Message",
    "OLLAMA_API_BASE",
    "SESSIONS_DIR",
    "SESSIONS_METADATA_FILE",
    "SYSTEM_PROMPTS_DIR",
    "__main__",
    "agentix",
    "api_client",
    "agentix_config",
    "assemble_prompts",
    "get_attachments",
    "get_file",
    "get_model",
    "get_models",
    "get_prompts",
    "get_session_history",
    "get_system_prompt",
    "get_user_prompt",
    "load_file",
    "main",
    "manage_sessions",
    #   "query_api",
    #   'summarize_user_prompt",
    "transform_ollama_tags_to_openai_engines",
    "trim_context",
]
