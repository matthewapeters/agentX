"""
Docstring for agentix.next_steps.respond_directly
"""

from agentix import AgentixConfig
from agentix.context.message import Message
from agentix.prompt_classification_response import NextStep


def respond_directly(
    args: AgentixConfig, next_step: NextStep, history: list[Message], max_tokens: int
) -> str:
    pass
