"""In-memory and disk-backed model metadata store."""

from __future__ import annotations

import json
import logging
import threading
from datetime import UTC, datetime
from pathlib import Path

from shared.providers.base import ILLMServiceProvider
from shared.providers.constants import FALLBACK_CONTEXT_WINDOW

logger = logging.getLogger(__name__)


class ModelMetadataStore:
    """Stores model capacities and metadata for fast runtime lookups.

    Attributes:
        populated: :class:`threading.Event` that is set when the first
            successful :meth:`populate` call completes.  Callers that want
            to wait for the background population to finish can use this
            event::

                store.populated.wait(timeout=5.0)
    """

    def __init__(self, provider: ILLMServiceProvider, cache_path: Path) -> None:
        """Initialise the store.

        Args:
            provider: Backend that can enumerate models and return metadata.
            cache_path: File system path for the on-disk JSON cache.
        """
        self._provider = provider
        self._cache_path = cache_path
        self._lock = threading.Lock()
        self._capacities: dict[str, int] = {}
        self._metadata: dict[str, dict[str, str | int]] = {}
        # Set once a populate() call has completed, whether successful or not.
        self.populated: threading.Event = threading.Event()
        self.population_failed: threading.Event = threading.Event()

    def populate(self, force: bool = False) -> None:
        """Populate store from provider and cache.

        If cache model set matches provider model set, in-memory state is
        restored from disk without per-model metadata calls.  Sets
        :attr:`populated` on completion.

        Args:
            force: When ``True``, bypass the cache-match check and re-fetch
                every model from the provider.
        """
        self.population_failed.clear()
        try:
            provider_models = sorted({m for m in self._provider.list_models() if m})
            cached = self._load_cache()

            if not force and cached and self._cache_models_match(cached, provider_models):
                self._load_from_cache(cached)
                self.populated.set()
                return

            # Unified parse: derive existing data from cache once.
            existing_capacities, existing_metadata = self._parse_cache_data(cached) if cached else ({}, {})

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
        except Exception:
            logger.exception("ModelMetadataStore.populate failed")
            self.population_failed.set()
        finally:
            self.populated.set()

    def invalidate(self, model_name: str | None = None) -> None:
        """Invalidate one model entry (or all entries) and re-populate.

        The re-population runs on a background daemon thread so this method
        returns immediately.  :attr:`populated` is cleared before the
        re-population starts and re-set when it completes.

        Args:
            model_name: Name of the model to evict.  Pass ``None`` to
                invalidate the entire store.
        """
        with self._lock:
            if model_name is None:
                self._capacities.clear()
                self._metadata.clear()
            else:
                self._capacities.pop(model_name, None)
                self._metadata.pop(model_name, None)
        self.populated.clear()
        threading.Thread(target=self.populate, daemon=True).start()

    def get_context_length(self, model_name: str) -> int:
        """Return context capacity for ``model_name`` with safe fallback.

        Args:
            model_name: Model identifier to look up.

        Returns:
            Token capacity, or :data:`~shared.providers.constants.FALLBACK_CONTEXT_WINDOW`
            when the model is unknown.
        """
        with self._lock:
            value = self._capacities.get(model_name)
            if value is None and ":" not in model_name:
                # Ollama stores models with an implicit ":latest" tag; try that.
                value = self._capacities.get(f"{model_name}:latest")
        if value is None:
            logger.warning("Model '%s' missing from metadata store; using fallback", model_name)
            return FALLBACK_CONTEXT_WINDOW
        return value

    def get_metadata(self, model_name: str) -> dict[str, str | int]:
        """Return metadata for ``model_name`` or empty dict.

        Args:
            model_name: Model identifier to look up.

        Returns:
            Metadata dict, or ``{}`` when the model is unknown.
        """
        with self._lock:
            result = self._metadata.get(model_name)
            if result is None and ":" not in model_name:
                result = self._metadata.get(f"{model_name}:latest")
            return dict(result) if result else {}

    def model_names(self) -> list[str]:
        """Return sorted model names currently present in the store."""
        with self._lock:
            return sorted(self._capacities.keys())

    def save_cache(self) -> None:
        """Persist current store state to disk.

        Uses :attr:`~shared.providers.base.ILLMServiceProvider.provider_id` to
        tag the cache file so multi-provider setups remain distinguishable.
        """
        with self._lock:
            payload = {
                "provider": self._provider.provider_id,
                "models": sorted(self._capacities.keys()),
                "capacities": dict(self._capacities),
                "metadata": dict(self._metadata),
                "updated_at": datetime.now(UTC).isoformat(),
            }
        self._cache_path.parent.mkdir(parents=True, exist_ok=True)
        self._cache_path.write_text(json.dumps(payload, indent=2), encoding="utf-8")

    def _load_cache(self) -> dict | None:
        """Read and parse the on-disk cache file.

        Returns:
            Parsed JSON dict, or ``None`` if the file is missing or corrupt.
        """
        if not self._cache_path.exists():
            return None
        try:
            return json.loads(self._cache_path.read_text(encoding="utf-8"))
        except Exception as exc:
            logger.warning("Could not read model metadata cache '%s': %s", self._cache_path, exc)
            return None

    @staticmethod
    def _cache_models_match(cached: dict, provider_models: list[str]) -> bool:
        """Return ``True`` when the cached model set matches ``provider_models``."""
        cached_models = sorted({str(m) for m in cached.get("models", []) if m})
        return cached_models == provider_models

    @staticmethod
    def _read_capacities(cached: dict) -> dict[str, int]:
        """Extract the ``capacities`` mapping from a raw cache dict."""
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
        """Extract the ``metadata`` mapping from a raw cache dict."""
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

    @classmethod
    def _parse_cache_data(cls, cached: dict) -> tuple[dict[str, int], dict[str, dict[str, str | int]]]:
        """Parse a raw cache dict into ``(capacities, metadata)`` dicts.

        This single entry-point eliminates the duplication between the
        full-cache-hit path (:meth:`_load_from_cache`) and the partial-hit
        path inside :meth:`populate`.

        Args:
            cached: Raw JSON dict loaded from the cache file.

        Returns:
            Tuple of ``(capacities, metadata)`` extracted from *cached*.
        """
        return cls._read_capacities(cached), cls._read_metadata(cached)

    def _load_from_cache(self, cached: dict) -> None:
        """Restore in-memory state from *cached* via :meth:`_parse_cache_data`."""
        capacities, metadata = self._parse_cache_data(cached)
        with self._lock:
            self._capacities = capacities
            self._metadata = metadata
