"""
Docstring for src.agentx.context
"""
from datetime import datetime
import json

from .message import Message

class Context:
    def __init__(self):
      self.messages: list[Message] = []  # List to hold context messages

    def add_message(self, ts: datetime, message: Message) -> None:
        """
        Add a new message to the context.
        """
        self.list.append((ts, message))

    def get_messages(self):
        """
        get_messages
        
        Use this method to convert the Context object to a JSON string.
        
        Only enabled messages are included in the output.
        
        :param self: Description
        """
        return json.dumps([m for m in self.messages if m.enabled])
