"""Ollama-backed implementation of ``ILLMServiceProvider``."""

from __future__ import annotations

import logging
from typing import Any

import requests

from agentix.constants import FALLBACK_CONTEXT_WINDOW, OLLAMA_MODELS_ENDPOINT, OLLAMA_SHOW_ENDPOINT

from .base import ILLMServiceProvider

logger = logging.getLogger(__name__)


class OllamaServiceProvider(ILLMServiceProvider):
    """Adapter around Ollama HTTP endpoints used by AgentX."""

    def __init__(self, host: str) -> None:
        self._host = self._normalize_host(host)

    @staticmethod
    def _normalize_host(host: str) -> str:
        if not host:
            return "http://localhost:11434"
        if host.startswith("http://") or host.startswith("https://"):
            return host.rstrip("/")
        return f"http://{host}".rstrip("/")

    def list_models(self) -> list[str]:
        """Return available model names from ``/api/tags``."""
        try:
            response = requests.get(f"{self._host}{OLLAMA_MODELS_ENDPOINT}", timeout=10)
            response.raise_for_status()
            payload = response.json()
            models = payload.get("models", [])
            return [str(item.get("name", "")).strip() for item in models if item.get("name")]
        except Exception as exc:
            logger.warning("Ollama list_models failed: %s", exc)
            return []

    def get_context_length(self, model_name: str) -> int:
        """Return context length for ``model_name`` with fallback on failure."""
        payload = self._show_payload(model_name)
        model_info = payload.get("model_info", {}) if isinstance(payload, dict) else {}
        if not isinstance(model_info, dict):
            logger.warning("Ollama show returned non-dict model_info for model '%s'", model_name)
            return FALLBACK_CONTEXT_WINDOW

        candidates = ["llama.context_length", "context_length", "num_ctx"]
        for key in candidates:
            value = model_info.get(key)
            parsed = self._to_int(value)
            if parsed is not None:
                logger.debug("Resolved context length for model '%s' from key '%s'", model_name, key)
                return parsed

        for key, value in model_info.items():
            if str(key).endswith(".context_length"):
                parsed = self._to_int(value)
                if parsed is not None:
                    logger.debug("Resolved context length for model '%s' from key '%s'", model_name, key)
                    return parsed

        logger.warning("No context-length key found for model '%s'; using fallback", model_name)
        return FALLBACK_CONTEXT_WINDOW

    def get_model_metadata(self, model_name: str) -> dict[str, str | int]:
        """Return display metadata from ``/api/show`` details block."""
        payload = self._show_payload(model_name)
        details = payload.get("details", {}) if isinstance(payload, dict) else {}
        if not isinstance(details, dict):
            return {}

        metadata: dict[str, str | int] = {}
        parameter_size = details.get("parameter_size")
        family = details.get("family")
        if isinstance(parameter_size, (str, int)):
            metadata["parameter_size"] = parameter_size
        if isinstance(family, (str, int)):
            metadata["family"] = family
        return metadata

    def _show_payload(self, model_name: str) -> dict[str, Any]:
        try:
            response = requests.post(
                f"{self._host}{OLLAMA_SHOW_ENDPOINT}",
                json={"model": model_name},
                timeout=10,
            )
            response.raise_for_status()
            payload = response.json()
            if isinstance(payload, dict):
                return payload
        except Exception as exc:
            logger.warning("Ollama show failed for model '%s': %s", model_name, exc)
        return {}

    @staticmethod
    def _to_int(value: Any) -> int | None:
        if isinstance(value, bool):
            return None
        if isinstance(value, int):
            return value
        if isinstance(value, float):
            return int(value)
        if isinstance(value, str):
            stripped = value.strip()
            if stripped.isdigit():
                return int(stripped)
        return None
