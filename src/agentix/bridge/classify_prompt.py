from shared.models.context import Context
from agentix.context.sessions import assemble_classification_prompt
from agentix.prompt_classification_response import (
    Intent,
    NextStep,
    PromptClassificationResponse,
)
from agentix.api_client import query_classification


def classify_prompt(
        config,
        prompt: str,
        context: Context,
        history,
        max_tokens: int = 500
        ) -> PromptClassificationResponse:
    """
    Classify user intent before processing.

    Analyzes the user's prompt to determine:
    - Intent type (conversation, simple_action, complex_action, safety_issue)
    - Whether clarification is needed
    - Next step to take (respond_directly, single_tool, invoke_planner, escalate)

    Args:
        prompt: User's input text
        context: Current conversation context

    Returns:
        PromptClassificationResponse with intent and next_step
    """

    # Build classification prompt using Agentix logic
    classification_payload = assemble_classification_prompt(
        config,
        history,
        max_tokens,
    )

    # Query API for classification
    result = query_classification(config, classification_payload)

    # Parse result into PromptClassificationResponse
    return PromptClassificationResponse(
        intent=Intent[result.get("intent", "conversation")],
        needs_clarification=result.get("needs_clarification", False),
        missing_fields=result.get("missing_fields", []),
        reasoning_summary=result.get("reasoning_summary", ""),
        next_step=NextStep[result.get("next_step", "respond_directly")],
    )
