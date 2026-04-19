"""
agentix.query_payload
~~~~~~~~~~~~~~~~~~~~~
Request payload model for the Ollama /v1/chat/completions endpoint.
"""

from dataclasses import dataclass
from typing import Any

from .context.message import Message


@dataclass
class QueryPayload:
    """Structured request payload for the Ollama OpenAI-compatible chat endpoint.

    Attributes:
        model (str): Ollama model tag to use (e.g. ``"gpt-oss:latest"``).
        messages (list[Message]): Ordered conversation messages.
        temperature (float): Sampling temperature; defaults to ``0.7``.
        max_tokens (int | None): Token ceiling for the response; ``None`` means
            the model default applies.
        format (str | None): When set to ``"json"`` the payload is serialised
            with ``response_format: {"type": "json_object"}`` so the
            OpenAI-compatible endpoint constrains the model to JSON-only output.
    """

    def __init__(
        self,
        model: str,
        messages: list[Message],
        temperature: float = 0.7,
        max_tokens: int | None = None,
        format: str | None = None,  # noqa: A002  (shadows built-in intentionally)
    ) -> None:
        """Initialise a QueryPayload.

        Args:
            model (str): Ollama model tag.
            messages (list[Message]): Conversation messages already converted to
                dicts by the caller (``assemble_prompts``).
            temperature (float): Sampling temperature.  Defaults to ``0.7``.
            max_tokens (int | None): Maximum response tokens.  ``None`` means
                the model default is used.
            format (str | None): Set to ``"json"`` to request structured JSON
                output via the OpenAI-compatible ``response_format`` field.
        """
        self.model = model
        self.messages = messages
        self.temperature = temperature
        self.max_tokens = max_tokens
        # Semantic label kept as "format" to match agentix_config field name;
        # serialised as response_format for the OpenAI-compat endpoint.
        self.format = format

    def to_dict(self) -> dict[str, Any]:
        """Convert this payload to a JSON-serialisable dictionary.

        The ``format`` flag is translated to the OpenAI-compatible
        ``response_format: {"type": "json_object"}`` key because the endpoint
        used is ``/v1/chat/completions``, which does not recognise the
        Ollama-native ``format`` key.

        Returns:
            dict[str, Any]: Payload ready for ``json.dumps()`` and
            ``requests.post()``.
        """
        payload: dict[str, Any] = {
            "model": self.model,
            "messages": self.messages,  # Already converted to dicts in assemble_prompts
            "temperature": self.temperature,
        }
        if self.max_tokens is not None:
            payload["max_tokens"] = self.max_tokens
        if self.format is not None:
            # The endpoint is /v1/chat/completions (OpenAI-compatible).
            # That API uses `response_format`, not the Ollama-native `format` key.
            payload["response_format"] = {"type": "json_object"}
        return payload
