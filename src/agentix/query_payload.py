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

    def __init__(self, model, messages: list[Message], temperature: float = 0.7):
        self.model = model
        self.messages = messages
        self.temperature = temperature
    
    def to_dict(self) -> dict:
        """Convert QueryPayload to dictionary for JSON serialization."""
        return {
            "model": self.model,
            "messages": self.messages,  # Already converted to dicts in assemble_prompts
            "temperature": self.temperature,
        }
