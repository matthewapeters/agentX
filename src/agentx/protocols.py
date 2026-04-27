"""Lightweight structural protocols for AgentX internal boundaries.

Using :class:`~typing.Protocol` instead of ``hasattr`` guards keeps
inter-module contracts explicit and type-safe while remaining decoupled from
concrete implementations.
"""

from __future__ import annotations

from typing import TYPE_CHECKING, Protocol, runtime_checkable

if TYPE_CHECKING:
    from shared.models.context import Context


@runtime_checkable
class IMeterSession(Protocol):
    """Protocol satisfied by :class:`~agentx.session.AgentXSession`.

    :class:`~agentx.streaming_controller.StreamingController` checks for this
    interface via :func:`isinstance` rather than ``hasattr`` guards so that the
    dependency is explicit and testable.
    """

    def _context_meter_payload(self, model_name: str | None = None) -> tuple[int, dict[str, int]]:
        """Return ``(max_tokens, breakdown)`` for the context meter.

        Args:
            model_name: Optional override for the active model name.

        Returns:
            Tuple of the context-window capacity and a per-band token breakdown.
        """
        ...

    def _schedule_meter_redraw(self, max_tokens: int, breakdown: dict[str, int]) -> None:
        """Schedule a context-meter redraw on the Tk main thread.

        Args:
            max_tokens: Total context-window capacity.
            breakdown: Per-band token counts.
        """
        ...

    def on_context_assembled(self, shared_context: "Context") -> None:
        """Update the context meter after shared context has been assembled.

        This method is called by
        :class:`~agentx.streaming_controller.StreamingController` immediately
        after :meth:`~agentx.session.AgentXSession._build_shared_context` so
        the meter can reflect the *actual* live context (including working
        memory and history) rather than just the session's own message list.

        Args:
            shared_context: The assembled :class:`~shared.models.context.Context`
                ready to be sent to the LLM.
        """
        ...
