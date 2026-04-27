"""In-memory and disk-backed model metadata store."""

from __future__ import annotations

import json
import logging
import threading
from datetime import UTC, datetime
from pathlib import Path

from agentix.constants import FALLBACK_CONTEXT_WINDOW

from .providers.base import ILLMServiceProvider

logger = logging.getLogger(__name__)


class ModelMetadataStore:
    """Stores model capacities and metadata for fast runtime lookups."""

    def __init__(self, provider: ILLMServiceProvider, cache_path: Path) -> None:
        self._provider = provider
        self._cache_path = cache_path
        self._lock = threading.Lock()
        self._capacities: dict[str, int] = {}
        self._metadata: dict[str, dict[str, str | int]] = {}

    def populate(self, force: bool = False) -> None:
        """Populate store from provider and cache.

        If cache model set matches provider model set, in-memory state is restored
        from disk without per-model metadata calls.
        """
        provider_models = sorted({m for m in self._provider.list_models() if m})
        cached = self._load_cache()

        if not force and cached and self._cache_models_match(cached, provider_models):
            self._load_from_cache(cached)
            return

        existing_capacities: dict[str, int] = {}
        existing_metadata: dict[str, dict[str, str | int]] = {}
        if cached:
            existing_capacities = self._read_capacities(cached)
            existing_metadata = self._read_metadata(cached)

        capacities: dict[str, int] = {}
        metadata: dict[str, dict[str, str | int]] = {}

        for model_name in provider_models:
            if not force and model_name in existing_capacities:
                capacities[model_name] = existing_capacities[model_name]
                metadata[model_name] = existing_metadata.get(model_name, {})
                continue

            context_length = self._provider.get_context_length(model_name)
            capacities[model_name] = context_length if context_length > 0 else FALLBACK_CONTEXT_WINDOW
            metadata[model_name] = self._provider.get_model_metadata(model_name)

        with self._lock:
            self._capacities = capacities
            self._metadata = metadata

        self.save_cache()

    def get_context_length(self, model_name: str) -> int:
        """Return context capacity for ``model_name`` with safe fallback."""
        with self._lock:
            value = self._capacities.get(model_name)
        if value is None:
            logger.warning("Model '%s' missing from metadata store; using fallback", model_name)
            return FALLBACK_CONTEXT_WINDOW
        return value

    def get_metadata(self, model_name: str) -> dict[str, str | int]:
        """Return metadata for ``model_name`` or empty dict."""
        with self._lock:
            return dict(self._metadata.get(model_name, {}))

    def model_names(self) -> list[str]:
        """Return sorted model names currently present in the store."""
        with self._lock:
            return sorted(self._capacities.keys())

    def save_cache(self) -> None:
        """Persist current store state to disk."""
        with self._lock:
            payload = {
                "provider": "ollama",
                "models": sorted(self._capacities.keys()),
                "capacities": dict(self._capacities),
                "metadata": dict(self._metadata),
                "updated_at": datetime.now(UTC).isoformat(),
            }
        self._cache_path.parent.mkdir(parents=True, exist_ok=True)
        self._cache_path.write_text(json.dumps(payload, indent=2), encoding="utf-8")

    def _load_cache(self) -> dict | None:
        if not self._cache_path.exists():
            return None
        try:
            return json.loads(self._cache_path.read_text(encoding="utf-8"))
        except Exception as exc:
            logger.warning("Could not read model metadata cache '%s': %s", self._cache_path, exc)
            return None

    @staticmethod
    def _cache_models_match(cached: dict, provider_models: list[str]) -> bool:
        cached_models = sorted({str(m) for m in cached.get("models", []) if m})
        return cached_models == provider_models

    @staticmethod
    def _read_capacities(cached: dict) -> dict[str, int]:
        raw = cached.get("capacities", {})
        result: dict[str, int] = {}
        if isinstance(raw, dict):
            for key, value in raw.items():
                if isinstance(value, int):
                    result[str(key)] = value
                elif isinstance(value, str) and value.isdigit():
                    result[str(key)] = int(value)
        return result

    @staticmethod
    def _read_metadata(cached: dict) -> dict[str, dict[str, str | int]]:
        raw = cached.get("metadata", {})
        result: dict[str, dict[str, str | int]] = {}
        if isinstance(raw, dict):
            for key, value in raw.items():
                if isinstance(value, dict):
                    result[str(key)] = {
                        str(meta_key): meta_val
                        for meta_key, meta_val in value.items()
                        if isinstance(meta_val, (str, int))
                    }
        return result

    def _load_from_cache(self, cached: dict) -> None:
        capacities = self._read_capacities(cached)
        metadata = self._read_metadata(cached)
        with self._lock:
            self._capacities = capacities
            self._metadata = metadata
