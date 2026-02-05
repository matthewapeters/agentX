"""
Docstring for agentix.next_steps.single_tool
"""

from agentix import AgentixConfig
from agentix.context.message import Message
from agentix.prompt_classification_response import NextStep


def single_tool(
    args: AgentixConfig, next_step: NextStep, history: list[Message], max_tokens: int
):
    """Handle the single tool next step."""
    pass
