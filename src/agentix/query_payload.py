"""
agentix.query_payload
"""

from dataclasses import dataclass

from .context import Message


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
