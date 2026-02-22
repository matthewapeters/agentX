
from shared.models.context import Context
from agentix.prompt_classification_response import (
    Intent,
    NextStep,
    PromptClassificationResponse,
)
from agentix.context.sessions import assemble_prompts
from agentix.constants import PROMPT_CLASSIFICATION, CLASSIFICATION_MAX_TOKENS
from agentix.api_client import query_classification
from agentix.agentix_config import AgentixConfig


def classify_prompt(
    config,
    prompt: str,
    context: Context,
    history,
    max_tokens: int = 500,
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

    # Use the classification prompt to ask the LLM to classify the user input
    # and determine next steps.  We do this for all user prompts.
    # We do not include system prompts or tool prompts in this classification step.
    classification_config = AgentixConfig()
    classification_config.model = config.classification_model or config.model
    classification_config.system = [PROMPT_CLASSIFICATION]
    classification_config.user = [prompt] if prompt else (config.user or [])
    classification_config.debug = config.debug
    classification_config.temperature = config.temperature

    response_max_tokens = (
        config.classification_max_tokens
        if config.classification_max_tokens is not None
        else CLASSIFICATION_MAX_TOKENS
    )

    effective_history = list(history) if history is not None else list(context.get_enabled_messages())

    # Build classification prompt using Agentix logic
    classification_payload = assemble_prompts(
        classification_config,
        effective_history,
        max_tokens,
        response_max_tokens=response_max_tokens,
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
