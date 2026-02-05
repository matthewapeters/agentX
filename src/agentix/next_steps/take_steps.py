"""
Docstring for agentix.next_steps.take_steps
"""

from agentix.agentix_config import AgentixConfig
from agentix.context import Message
from agentix.prompt_classification_response import NextStep

from . import escalate, invoke_planner, respond_directly, single_tool


def take_steps(
    args: AgentixConfig, next_step: NextStep, history: list[Message], max_tokens: int
):
    # Take next steps based on classification
    match next_step:
        case NextStep.escalate:
            escalate.escalate(args, next_step, history, max_tokens)
        case NextStep.invoke_planner:
            invoke_planner.invoke_planner(args, next_step, history, max_tokens)
        case NextStep.single_tool:
            single_tool.single_tool(args, next_step, history, max_tokens)
        case _:
            respond_directly.respond_directly(args, next_step, history, max_tokens)
