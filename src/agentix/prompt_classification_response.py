"""
agentix.prompt_classification_response
"""

from dataclasses import dataclass
from enum import Enum

Intent = Enum(
    "Intent",
    [
        "conversation",
        "simple_action",
        "complex_action",
        "safety_issue",
    ],
)

NextStep = Enum(
    "Next_Step",
    [
        "respond_directly",
        "single_tool",
        "invoke_planner",
        "escalate",
    ],
)


@dataclass
class PromptClassificationResponse:
    """
    Docstring for prompt_classification_response

        #  "intent": "conversation | simple_action | complex_action | safety_issue",
        #   "needs_clarification": boolean,
        #   "missing_fields": [ "list of missing info if any" ],
        #   "reasoning_summary": "brief explanation of the classification decision",
        #   "next_step": "respond_directly | single_tool | invoke_planner | escalate"
    """

    def __init__(
        self,
        intent: Intent,
        needs_clarification: bool,
        missing_fields: list,
        reasoning_summary: str,
        next_step: NextStep,
    ):
        self.intent: Intent = intent
        self.needs_clarification: bool = needs_clarification
        self.missing_fields: list[str] = missing_fields
        self.reasoning_summary: str = reasoning_summary
        self.next_step: NextStep = next_step
