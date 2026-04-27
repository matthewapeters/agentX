"""Provider-scoped constants for LLM service integrations.

These constants are used exclusively within the ``agentx.providers`` package
and ``agentx.model_metadata_store``.  Keeping them here removes the cross-tree
dependency on ``agentix.constants`` that previously existed in the provider
implementation modules.
"""

# Ollama HTTP endpoint paths
OLLAMA_MODELS_ENDPOINT: str = "/api/tags"
OLLAMA_SHOW_ENDPOINT: str = "/api/show"

# Fallback context-window size when a provider cannot resolve model metadata.
FALLBACK_CONTEXT_WINDOW: int = 4096
