"""Shared LLM provider abstractions and implementations.

These modules live under ``shared`` so both ``agentx`` and ``agentix`` can
consume them without introducing reverse import dependencies.
"""

from .base import ILLMServiceProvider
from .constants import FALLBACK_CONTEXT_WINDOW, OLLAMA_MODELS_ENDPOINT, OLLAMA_SHOW_ENDPOINT
from .ollama_provider import OllamaServiceProvider

__all__ = [
    "FALLBACK_CONTEXT_WINDOW",
    "ILLMServiceProvider",
    "OLLAMA_MODELS_ENDPOINT",
    "OLLAMA_SHOW_ENDPOINT",
    "OllamaServiceProvider",
]
