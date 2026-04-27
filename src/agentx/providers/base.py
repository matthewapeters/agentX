"""Provider interface for LLM service integrations."""

from typing import Protocol, runtime_checkable


@runtime_checkable
class ILLMServiceProvider(Protocol):
    """Common interface for model enumeration and model metadata lookups.

    Implementations must never raise from public methods and should return
    safe defaults when upstream calls fail.
    """

    def list_models(self) -> list[str]:
        """Return the available model names."""

    def get_context_length(self, model_name: str) -> int:
        """Return context-window token capacity for ``model_name``."""

    def get_model_metadata(self, model_name: str) -> dict[str, str | int]:
        """Return provider-specific metadata for ``model_name``."""
