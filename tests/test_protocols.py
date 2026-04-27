"""Unit tests for agentx.protocols (IMeterSession).

GIVEN concrete and incomplete implementations
WHEN isinstance is used with the runtime-checkable IMeterSession protocol
THEN structural compatibility is correctly detected.
"""

import pytest

from agentx.protocols import IMeterSession


class _FullImpl:
    """A class that fully implements the IMeterSession protocol."""

    def _context_meter_payload(self, model_name: str | None = None) -> tuple[int, dict[str, int]]:
        return (4096, {})

    def _schedule_meter_redraw(self, max_tokens: int, breakdown: dict[str, int]) -> None:
        pass

    def on_context_assembled(self, shared_context: object) -> None:
        pass


class _MissingMethod:
    """A class that is missing on_context_assembled."""

    def _context_meter_payload(self, model_name: str | None = None) -> tuple[int, dict[str, int]]:
        return (4096, {})

    def _schedule_meter_redraw(self, max_tokens: int, breakdown: dict[str, int]) -> None:
        pass


@pytest.mark.unit
def test_full_impl_is_imeter_session() -> None:
    """GIVEN a class with all required methods WHEN isinstance(obj, IMeterSession) THEN True."""
    obj = _FullImpl()
    assert isinstance(obj, IMeterSession)


@pytest.mark.unit
def test_missing_method_is_not_imeter_session() -> None:
    """GIVEN a class missing on_context_assembled WHEN isinstance check THEN False."""
    obj = _MissingMethod()
    assert not isinstance(obj, IMeterSession)


@pytest.mark.unit
def test_unrelated_object_is_not_imeter_session() -> None:
    """GIVEN a plain dict WHEN isinstance(obj, IMeterSession) THEN False."""
    assert not isinstance({}, IMeterSession)
