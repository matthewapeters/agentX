"""Unit tests for ModelMetadataStore.

GIVEN a model metadata store
WHEN startup population runs
THEN it should use cache when possible and fetch missing models only.
"""

from __future__ import annotations

import json
from pathlib import Path

import pytest

from agentx.model_metadata_store import ModelMetadataStore
from agentx.providers.constants import FALLBACK_CONTEXT_WINDOW


class FakeProvider:
    """Simple provider test double for deterministic store tests."""

    provider_id: str = "fake-provider"

    def __init__(self, models: list[str], capacities: dict[str, int]) -> None:
        self._models = models
        self._capacities = capacities
        self.context_calls: list[str] = []

    def list_models(self) -> list[str]:
        return list(self._models)

    def get_context_length(self, model_name: str) -> int:
        self.context_calls.append(model_name)
        return self._capacities.get(model_name, FALLBACK_CONTEXT_WINDOW)

    def get_model_metadata(self, model_name: str) -> dict[str, str | int]:
        return {"family": "test", "parameter_size": "7B", "name": model_name}


@pytest.mark.unit
def test_populate_fetches_all_without_cache(tmp_path: Path) -> None:
    """GIVEN no cache WHEN populate runs THEN all model capacities are fetched."""
    provider = FakeProvider(models=["a", "b"], capacities={"a": 1024, "b": 2048})
    store = ModelMetadataStore(provider=provider, cache_path=tmp_path / "cache.json")

    store.populate()

    assert store.get_context_length("a") == 1024
    assert store.get_context_length("b") == 2048
    assert provider.context_calls == ["a", "b"]


@pytest.mark.unit
def test_populate_uses_cache_when_model_set_matches(tmp_path: Path) -> None:
    """GIVEN matching cache WHEN populate runs THEN per-model fetch is skipped."""
    cache_path = tmp_path / "cache.json"
    cache_path.write_text(
        json.dumps(
            {
                "provider": "ollama",
                "models": ["a"],
                "capacities": {"a": 4096},
                "metadata": {"a": {"family": "test"}},
                "updated_at": "2026-04-26T12:00:00Z",
            }
        ),
        encoding="utf-8",
    )

    provider = FakeProvider(models=["a"], capacities={"a": 1024})
    store = ModelMetadataStore(provider=provider, cache_path=cache_path)

    store.populate()

    assert store.get_context_length("a") == 4096
    assert provider.context_calls == []


@pytest.mark.unit
def test_get_context_length_unknown_returns_fallback(tmp_path: Path) -> None:
    """GIVEN unknown model WHEN looked up THEN fallback context window is returned."""
    provider = FakeProvider(models=["a"], capacities={"a": 1024})
    store = ModelMetadataStore(provider=provider, cache_path=tmp_path / "cache.json")
    store.populate()

    assert store.get_context_length("missing") == FALLBACK_CONTEXT_WINDOW


@pytest.mark.unit
def test_populated_event_is_set_after_populate(tmp_path: Path) -> None:
    """GIVEN store not yet populated WHEN populate completes THEN .populated event is set."""
    provider = FakeProvider(models=["x"], capacities={"x": 8192})
    store = ModelMetadataStore(provider=provider, cache_path=tmp_path / "cache.json")

    assert not store.populated.is_set()
    store.populate()
    assert store.populated.is_set()


@pytest.mark.unit
def test_populated_event_set_even_on_provider_failure(tmp_path: Path) -> None:
    """GIVEN a provider that raises WHEN populate is called THEN .populated is set and failure is recorded.

    This ensures the background thread never leaves callers waiting indefinitely.
    """

    class BrokenProvider:
        provider_id = "broken"

        def list_models(self) -> list[str]:  # type: ignore[return]
            raise RuntimeError("network error")

        def get_context_length(self, model_name: str) -> int:
            return FALLBACK_CONTEXT_WINDOW

        def get_model_metadata(self, model_name: str) -> dict:
            return {}

    store = ModelMetadataStore(provider=BrokenProvider(), cache_path=tmp_path / "cache.json")
    store.populate()  # must not raise, must set event
    assert store.populated.is_set()
    assert store.population_failed.is_set()


@pytest.mark.unit
def test_population_failed_clears_after_successful_retry(tmp_path: Path) -> None:
    """GIVEN a failed populate followed by a successful one WHEN populate reruns THEN failure state is cleared."""

    class FlakyProvider:
        """Provider double that fails once and then succeeds."""

        provider_id = "flaky"

        def __init__(self) -> None:
            self._attempts = 0

        def list_models(self) -> list[str]:
            self._attempts += 1
            if self._attempts == 1:
                raise RuntimeError("first failure")
            return ["ok"]

        def get_context_length(self, model_name: str) -> int:
            return 2048

        def get_model_metadata(self, model_name: str) -> dict[str, str | int]:
            return {"family": "test"}

    store = ModelMetadataStore(provider=FlakyProvider(), cache_path=tmp_path / "cache.json")

    store.populate()
    assert store.population_failed.is_set()

    store.populate(force=True)

    assert store.populated.is_set()
    assert not store.population_failed.is_set()
    assert store.get_context_length("ok") == 2048


@pytest.mark.unit
def test_save_cache_uses_provider_id(tmp_path: Path) -> None:
    """GIVEN a fake provider with a custom provider_id WHEN save_cache runs THEN cache records that id."""
    provider = FakeProvider(models=["m"], capacities={"m": 2048})
    store = ModelMetadataStore(provider=provider, cache_path=tmp_path / "cache.json")
    store.populate()

    cached = json.loads((tmp_path / "cache.json").read_text())
    assert cached["provider"] == "fake-provider"


@pytest.mark.unit
def test_invalidate_clears_single_model(tmp_path: Path) -> None:
    """GIVEN populated store WHEN invalidate(model) is called THEN that model is evicted.

    The invalidated model returns FALLBACK_CONTEXT_WINDOW before re-population.
    """
    provider = FakeProvider(models=["a", "b"], capacities={"a": 1024, "b": 4096})
    store = ModelMetadataStore(provider=provider, cache_path=tmp_path / "cache.json")
    store.populate()
    assert store.get_context_length("a") == 1024

    # After invalidation the entry is gone; wait for background re-populate.
    store.invalidate("a")
    store.populated.wait(timeout=5.0)
    # Re-fetched from provider (context_calls should contain 'a' a second time).
    assert store.get_context_length("a") == 1024


@pytest.mark.unit
def test_invalidate_all_clears_store(tmp_path: Path) -> None:
    """GIVEN populated store WHEN invalidate() (no arg) is called THEN all entries are cleared."""
    provider = FakeProvider(models=["a"], capacities={"a": 999})
    store = ModelMetadataStore(provider=provider, cache_path=tmp_path / "cache.json")
    store.populate()

    store.invalidate()  # clear all
    store.populated.wait(timeout=5.0)
    # After re-population the value is restored.
    assert store.get_context_length("a") == 999


@pytest.mark.unit
def test_parse_cache_data_returns_both_dicts(tmp_path: Path) -> None:
    """GIVEN a raw cache dict WHEN _parse_cache_data is called THEN both capacities and metadata are returned."""
    raw = {
        "capacities": {"m": 1234},
        "metadata": {"m": {"family": "test"}},
    }
    capacities, metadata = ModelMetadataStore._parse_cache_data(raw)
    assert capacities == {"m": 1234}
    assert metadata == {"m": {"family": "test"}}


@pytest.mark.unit
def test_populate_uses_cached_capacity_for_partial_hit(tmp_path: Path) -> None:
    """GIVEN cached capacities for a subset WHEN populate runs THEN cached values are reused and missing ones are fetched."""
    cache_path = tmp_path / "cache.json"
    cache_path.write_text(
        json.dumps(
            {
                "models": ["a", "b", "c"],
                "capacities": {"a": 1024},
                "metadata": {"a": {"family": "cached"}},
            }
        ),
        encoding="utf-8",
    )
    provider = FakeProvider(models=["a", "b"], capacities={"a": 9999, "b": 2048})
    store = ModelMetadataStore(provider=provider, cache_path=cache_path)

    store.populate()

    assert store.get_context_length("a") == 1024
    assert store.get_context_length("b") == 2048
    assert provider.context_calls == ["b"]


@pytest.mark.unit
def test_get_metadata_returns_copy_and_model_names_are_sorted(tmp_path: Path) -> None:
    """GIVEN populated metadata WHEN retrieved THEN returned metadata is copied and model names are sorted."""
    provider = FakeProvider(models=["b", "a"], capacities={"a": 1024, "b": 2048})
    store = ModelMetadataStore(provider=provider, cache_path=tmp_path / "cache.json")
    store.populate()

    metadata = store.get_metadata("a")
    metadata["family"] = "changed"

    assert store.get_metadata("a")["family"] == "test"
    assert store.model_names() == ["a", "b"]


@pytest.mark.unit
def test_load_cache_returns_none_for_corrupt_json(tmp_path: Path) -> None:
    """GIVEN corrupt cache JSON WHEN _load_cache is called THEN None is returned."""
    provider = FakeProvider(models=[], capacities={})
    cache_path = tmp_path / "cache.json"
    cache_path.write_text("{bad json", encoding="utf-8")
    store = ModelMetadataStore(provider=provider, cache_path=cache_path)

    assert store._load_cache() is None


@pytest.mark.unit
def test_read_capacities_and_metadata_filter_invalid_values() -> None:
    """GIVEN mixed raw cache values WHEN parsed THEN only valid capacities and metadata entries survive."""
    raw = {
        "capacities": {"a": 1024, "b": "2048", "c": "bad"},
        "metadata": {"a": {"family": "ok", "count": 7, "bad": []}, "b": "bad"},
    }

    assert ModelMetadataStore._read_capacities(raw) == {"a": 1024, "b": 2048}
    assert ModelMetadataStore._read_metadata(raw) == {"a": {"family": "ok", "count": 7}}


@pytest.mark.unit
def test_cache_models_match_uses_sorted_non_empty_values() -> None:
    """GIVEN cached models with duplicates and empties WHEN compared THEN only sorted non-empty values are considered."""
    cached = {"models": ["b", "", "a", "a"]}

    assert ModelMetadataStore._cache_models_match(cached, ["a", "b"])


@pytest.mark.unit
@pytest.mark.parametrize(
    "lookup_name,stored_name,expected_tokens",
    [
        ("gpt-oss", "gpt-oss:latest", 32768),
        ("llama3.2", "llama3.2:latest", 131072),
        ("gpt-oss:latest", "gpt-oss:latest", 32768),  # exact match still works
        ("completely-unknown", "gpt-oss:latest", None),  # no match → fallback
    ],
    ids=["bare-name-latest", "bare-name-llama", "exact-tagged-name", "no-match-fallback"],
)
def test_get_context_length_latest_tag_fallback(
    tmp_path,
    lookup_name: str,
    stored_name: str,
    expected_tokens: int | None,
) -> None:
    """GIVEN Ollama stores models with ':latest' tags
    WHEN get_context_length is called with the bare model name (no tag)
    THEN the correct capacity is returned without a fallback warning.

    Ollama always appends ':latest' when no explicit tag is given, so
    'gpt-oss' in agentx.toml must resolve to 'gpt-oss:latest' in the store.

    Parameterized cases:
    - bare-name-latest: 'gpt-oss' resolves to 'gpt-oss:latest'
    - bare-name-llama: 'llama3.2' resolves to 'llama3.2:latest'
    - exact-tagged-name: 'gpt-oss:latest' resolves via direct match
    - no-match-fallback: completely unknown model returns FALLBACK_CONTEXT_WINDOW
    """
    provider = FakeProvider(models=[stored_name], capacities={stored_name: expected_tokens or 8192})
    store = ModelMetadataStore(provider=provider, cache_path=tmp_path / "cache.json")
    store.populate()

    result = store.get_context_length(lookup_name)
    if expected_tokens is not None:
        assert result == expected_tokens
    else:
        assert result == FALLBACK_CONTEXT_WINDOW


@pytest.mark.unit
@pytest.mark.parametrize(
    "lookup_name,stored_name",
    [
        ("gpt-oss", "gpt-oss:latest"),
        ("llama3.2", "llama3.2:latest"),
    ],
    ids=["gpt-oss-latest", "llama3.2-latest"],
)
def test_get_metadata_latest_tag_fallback(tmp_path, lookup_name: str, stored_name: str) -> None:
    """GIVEN Ollama stores models with ':latest' tags
    WHEN get_metadata is called with the bare model name
    THEN the correct metadata dict is returned.

    Mirrors the ':latest' tag fallback behaviour of get_context_length.
    """
    provider = FakeProvider(models=[stored_name], capacities={stored_name: 8192})
    store = ModelMetadataStore(provider=provider, cache_path=tmp_path / "cache.json")
    store.populate()

    metadata = store.get_metadata(lookup_name)
    assert metadata.get("family") == "test"
