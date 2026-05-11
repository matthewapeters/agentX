import logging
import re
from datetime import datetime
from typing import Optional

from agentix.agentix_config import AgentixConfig
from agentix.api_client import query_classification
from agentix.bridge.prompt_assembly import assemble_prompts
from agentix.constants import CLASSIFICATION_MAX_TOKENS, PROMPT_CLASSIFICATION
from agentix.prompt_classification_response import (
    Intent,
    NextStep,
    PromptClassificationResponse,
)
from shared.models.context import Context
from shared.models.working_memory import WorkingMemory

logger = logging.getLogger("agentix.classification")


_VIBE_EDITOR_INTENT_PATTERNS: tuple[re.Pattern[str], ...] = (
    re.compile(r"\b(?:open|edit)\b.*\b(?:vibe(?:\s*editor)?|neovim|nvim|vim)\b", re.IGNORECASE),
    re.compile(r"\b(?:vibe(?:\s*editor)?|neovim|nvim|vim)\b.*\b(?:open|edit)\b", re.IGNORECASE),
    re.compile(r"\bopen\b.*\bin\b.*\b(?:editor|vibe(?:\s*editor)?)\b", re.IGNORECASE),
)


def _route_vibe_editor_intent(prompt: str) -> Optional[PromptClassificationResponse]:
    """Deterministically route explicit vibe-editor intents to single-tool execution.

    Args:
        prompt: User prompt text.

    Returns:
        PromptClassificationResponse | None: Forced route when an explicit vibe-editor
        intent is detected; ``None`` otherwise.
    """
    prompt_text = (prompt or "").strip()
    if not prompt_text:
        return None

    for pattern in _VIBE_EDITOR_INTENT_PATTERNS:
        if pattern.search(prompt_text):
            return PromptClassificationResponse(
                intent=Intent.simple_action,
                needs_clarification=False,
                missing_fields=[],
                reasoning_summary="Explicit vibe-editor request routed to editor tool path.",
                next_step=NextStep.single_tool,
            )
    return None


def _format_working_memory_for_classification(wm: WorkingMemory) -> str:
    """
    Format Working Memory facts for classification prompt.

    Args:
        wm: WorkingMemory instance with session facts

    Returns:
        Formatted string with <working_memory> block
    """
    lines = ["<working_memory>"]
    for fact in wm.get_enabled_facts():
        owner_icon = fact.owner.icon
        # Format value: if it's a string, use as-is; otherwise convert to string
        value_str = fact.value if isinstance(fact.value, str) else str(fact.value)
        lines.append(f"{owner_icon} {fact.key}: {value_str}")
    lines.append("</working_memory>")
    return "\n".join(lines)


def classify_prompt(
    config,
    prompt: str,
    context: Context,
    history,
    max_tokens: int = 500,
    working_memory: Optional[WorkingMemory] = None,
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
        history: Conversation history
        max_tokens: Maximum tokens for classification
        working_memory: Optional WorkingMemory instance for context

    Returns:
        PromptClassificationResponse with intent and next_step
    """
    start_time = datetime.now()

    # LOG: Classification attempt started
    logger.info(
        "Classification started",
        extra={
            "prompt_preview": prompt[:100] if prompt else None,
            "prompt_length": len(prompt) if prompt else 0,
            "context_message_count": len(context.get_enabled_messages()) if context else 0,
            "has_working_memory": working_memory is not None,
            "wm_fact_count": len(working_memory.all_facts()) if working_memory else 0,
        },
    )

    vibe_editor_route = _route_vibe_editor_intent(prompt)
    if vibe_editor_route is not None:
        logger.info(
            "Classification short-circuited by vibe-editor intent routing",
            extra={
                "intent": vibe_editor_route.intent.name,
                "next_step": vibe_editor_route.next_step.name,
                "reasoning": vibe_editor_route.reasoning_summary,
                "duration_ms": (datetime.now() - start_time).total_seconds() * 1000,
            },
        )
        return vibe_editor_route

    # Use the classification prompt to ask the LLM to classify the user input
    # and determine next steps.  We do this for all user prompts.
    # We do not include system prompts or tool prompts in this classification step.
    classification_config = AgentixConfig()
    classification_config.model = config.classification_model or config.model
    classification_config.system = [PROMPT_CLASSIFICATION]
    # Inherit the prompt directory from the caller's config so PromptLoader
    # resolves prompt_classification.md from the project's system_prompts/
    # folder rather than falling back to the default ~/.agentix/system_prompts/
    # path (which typically does not contain the file, causing the model to
    # receive an empty system block and respond in its native persona).
    if hasattr(config, "system_prompts_dir") and config.system_prompts_dir:
        classification_config.system_prompts_dir = config.system_prompts_dir

    # Inject Working Memory into user prompt if available
    enhanced_prompt = prompt
    if working_memory and working_memory.all_facts():
        wm_context = _format_working_memory_for_classification(working_memory)
        enhanced_prompt = f"{wm_context}\n\n{prompt}"
        logger.debug(
            "Working Memory injected into classification prompt",
            extra={"wm_fact_count": len(working_memory.all_facts())},
        )

    classification_config.user = [enhanced_prompt] if enhanced_prompt else (config.user or [])
    classification_config.debug = config.debug
    classification_config.temperature = config.temperature

    logger.debug(
        "Classification config prepared",
        extra={
            "model": classification_config.model,
            "temperature": classification_config.temperature,
            "backend": getattr(config, "classification_backend", "ollama"),
        },
    )

    response_max_tokens = (
        config.classification_max_tokens if config.classification_max_tokens is not None else CLASSIFICATION_MAX_TOKENS
    )

    effective_history = list(history) if history is not None else list(context.get_enabled_messages())

    # Filter to only enabled messages — disabled messages (e.g. soft-deleted turns) must
    # not influence classification regardless of how history was assembled.
    effective_history = [
        msg for msg in effective_history if (msg.enabled if hasattr(msg, "enabled") else msg.get("enabled", True))
    ]

    # Filter to user/assistant roles only.  System messages (working memory
    # injection, main-agent system prompt) must not be forwarded to the
    # classification model: they carry the agent's identity and tool-use context
    # which compete with the classification system prompt that assemble_prompts
    # will inject, causing the model to answer in its agent persona instead of
    # producing the required JSON schema.
    _CLASSIFY_ROLES: set[str] = {"user", "assistant"}
    effective_history = [
        msg
        for msg in effective_history
        if (msg.role.value if hasattr(msg, "role") and hasattr(msg.role, "value") else msg.get("role", ""))
        in _CLASSIFY_ROLES
    ]

    # Limit history to the last few *active* turns so that the classification system prompt
    # is placed close to the head of the message list rather than being buried after a long
    # conversation.  Without this, models that have seen many prior exchanges tend to
    # continue the conversational tone and add natural-language preamble around the
    # required JSON, causing json.loads to fail.  Two prior messages (one exchange) are
    # enough to give the model context for back-references like "as discussed".
    CLASSIFICATION_HISTORY_LIMIT = 2
    if len(effective_history) > CLASSIFICATION_HISTORY_LIMIT:
        effective_history = effective_history[-CLASSIFICATION_HISTORY_LIMIT:]

    logger.debug("History filtered for classification", extra={"effective_history_length": len(effective_history)})

    # Build classification prompt using Agentix logic
    # Set response_format to 'json' to force JSON-only output from Ollama
    classification_config.response_format = "json"

    classification_payload = assemble_prompts(
        classification_config,
        effective_history,
        max_tokens,
        response_max_tokens=response_max_tokens,
    )

    # Query API for classification
    try:
        result = query_classification(config, classification_payload)

        # LOG: Raw LLM result before parsing
        logger.info(
            "Classification raw result",
            extra={
                "result": result,
                "result_keys": list(result.keys()) if isinstance(result, dict) else None,
                "duration_ms": (datetime.now() - start_time).total_seconds() * 1000,
            },
        )

        # Validate required fields are present and non-empty
        if not isinstance(result, dict):
            raise ValueError(f"Classification result must be a dict, got {type(result).__name__}: {result}")

        # Check for required fields
        # intent may be empty when needs_clarification is True (LLM legitimately cannot determine intent)
        needs_clarification = result.get("needs_clarification", False)
        required_fields = ["next_step", "reasoning_summary"]
        if not needs_clarification:
            required_fields.append("intent")
        missing = [f for f in required_fields if f not in result or not result[f]]
        if missing:
            logger.error(
                "Classification missing required fields",
                extra={
                    "missing_fields": missing,
                    "result": result,
                    "result_repr": repr(result),
                },
            )
            raise ValueError(
                f"Classification returned incomplete JSON. Missing or empty fields: {missing}. "
                f"Full result: {result}. This usually means the LLM failed to generate proper classification. "
                f"Try: (1) switch to a more reliable model like gpt-oss:latest, "
                f"(2) add format='json' to force JSON output, or (3) lower temperature to 0.1-0.3."
            )

        # Parse result into PromptClassificationResponse
        # When needs_clarification is True, intent may be empty — fall back to "conversation"
        raw_intent = result.get("intent") or "conversation"
        response = PromptClassificationResponse(
            intent=Intent[raw_intent],  # Raises KeyError if invalid non-empty value
            needs_clarification=result.get("needs_clarification", False),
            missing_fields=result.get("missing_fields", []),
            reasoning_summary=result["reasoning_summary"],  # No default - required
            next_step=NextStep[result["next_step"]],  # No default - will raise KeyError if invalid
        )

        # LOG: Final classification decision
        logger.info(
            "Classification complete",
            extra={
                "intent": response.intent.name,
                "next_step": response.next_step.name,
                "needs_clarification": response.needs_clarification,
                "missing_fields": response.missing_fields,
                "reasoning": response.reasoning_summary,
                "total_duration_ms": (datetime.now() - start_time).total_seconds() * 1000,
            },
        )

        return response

    except KeyError as e:
        # Enum lookup failed — invalid intent/next_step value from LLM
        logger.error(
            "Classification enum parse error",
            extra={
                "error": str(e),
                "raw_result": result,
                "valid_intents": [i.name for i in Intent],
                "valid_next_steps": [n.name for n in NextStep],
            },
            exc_info=True,
        )
        raise
    except Exception as e:
        logger.error(
            "Classification failed",
            extra={
                "error": str(e),
                "error_type": type(e).__name__,
            },
            exc_info=True,
        )
        raise
