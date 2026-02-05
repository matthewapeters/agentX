"""
Docstring for agentix.next_steps.invoke_planner
"""

from agentix import AgentixConfig
from agentix.api_client import query_api
from agentix.context import Message
from agentix.context.sessions import assemble_prompts
from agentix.next_steps.take_steps import NextStep

INVOKE_PLANNER_PROMPT = "invoke_planner"


def invoke_planner(
    args: AgentixConfig, next_step: NextStep, history: list[Message], max_tokens: int
) -> str:
    """
    Docstring for invoke_planner

    invoke the LLM for a structured plan to address the user's prompt

    The goal is produce a machine-readable todo-list with tool-calls

    params:
    args: AgentixConfig User prompt and settings
    next_step: NextStep The LLM's directions for how to handle the user request
    history: list[Message] The conversation history between the user and the LLM
    max_tokens: int The max tokens allowed in the context
    """
    planner_args: AgentixConfig = AgentixConfig()
    planner_args.debug = args.debug
    planner_args.system = [INVOKE_PLANNER_PROMPT]
    planner_args.user = args.user
    planner_args.file_path = args.file_path
    planner_args.model = args.model

    qp = assemble_prompts(planner_args, history, max_tokens)
    result = query_api(planner_args, qp)
