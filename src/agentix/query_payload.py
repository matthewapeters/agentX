"""
agentix.query_payload
"""

from dataclasses import dataclass

from .context.message import Message


@dataclass
class QueryPayload:
    """
    Docstring for QueryPayload

    :var params: Description
    """

    def __init__(
        self,
        model,
        messages: list[Message],
        temperature: float = 0.7,
        max_tokens: int | None = None,
    ):
        self.model = model
        self.messages = messages
        self.temperature = temperature
        self.max_tokens = max_tokens
    
    def to_dict(self) -> dict:
        """Convert QueryPayload to dictionary for JSON serialization."""
        payload = {
            "model": self.model,
            "messages": self.messages,  # Already converted to dicts in assemble_prompts
            "temperature": self.temperature,
        }
        if self.max_tokens is not None:
            payload["max_tokens"] = self.max_tokens
        return payload
