"""agentix.constants.py"""

import os

AGENTIX_HOME = os.getenv("AGENTIX_HOME", os.path.expanduser("~/.agentix"))

# Constants for Agentix CLI

# Token limits
MAX_TOKENS = 4096

# Directory paths
SYSTEM_PROMPTS_DIR = f"{AGENTIX_HOME}/system_prompts/"
SESSIONS_DIR = f"{AGENTIX_HOME}/sessions/"
SESSIONS_METADATA_FILE = f"{AGENTIX_HOME}/agentix_sessions.json"

# API configuration
OLLAMA_API_BASE = "http://localhost:11434"
OLLAMA_MODELS_ENDPOINT = "/api/tags"
OLLAMA_CHAT_ENDPOINT = "/v1/chat/completions"

# Default values
DEFAULT_TEMPERATURE = 0.2
DEFAULT_SESSION_ID = "agentix_session"
CONTINUE_SESSION_ID = "__continue"

# All user prompts are classified before processing
PROMPT_CLASSIFICATION = "prompt_classification"
