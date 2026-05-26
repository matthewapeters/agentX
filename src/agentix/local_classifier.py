"""Local intent classifier using SetFit embeddings."""

from __future__ import annotations

from functools import lru_cache
from typing import Any

import numpy as np

from .prompt_classification_response import Intent, NextStep

_INTENT_TO_NEXT_STEP = {
    Intent.conversation.name: NextStep.respond_directly.name,
    Intent.simple_action.name: NextStep.single_tool.name,
    Intent.complex_action.name: NextStep.invoke_planner.name,
    Intent.safety_issue.name: NextStep.escalate.name,
}

_CLASS_LABELS = {
    "chat": "General conversation, Q&A, and helpful replies.",
    "code": "Programming help, debugging, and code changes.",
    "data": "Data analysis, SQL, metrics, charts, and datasets.",
    "system": "System operations, infrastructure, access, or security issues.",
    "planning": "Multi-step planning, coordination, or project strategy.",
}

_CLASS_TO_INTENT = {
    "chat": Intent.conversation.name,
    "code": Intent.simple_action.name,
    "data": Intent.complex_action.name,
    "system": Intent.safety_issue.name,
    "planning": Intent.complex_action.name,
}


@lru_cache(maxsize=2)
def _get_setfit_model(model_id: str):
    from setfit import SetFitModel

    return SetFitModel.from_pretrained(model_id)


@lru_cache(maxsize=2)
def _get_label_embeddings(model_id: str) -> np.ndarray:
    model = _get_setfit_model(model_id)
    label_texts = [f"{label}: {desc}" for label, desc in _CLASS_LABELS.items()]
    embeddings = model.model_body.encode(label_texts, normalize_embeddings=True)
    return np.asarray(embeddings, dtype=np.float32)


def classify_intent_with_torch(
    prompt: str,
    model_name: str | None,
    device: int | None = None,
) -> dict[str, Any]:
    """Classify intent using SetFit embeddings and similarity.

    Args:
        prompt: User prompt text to classify.
        model_name: Sentence embedding model id. Defaults to bge-small-en-v1.5.
        device: Unused for SetFit embedding inference.

    Returns:
        Dict compatible with PromptClassificationResponse.
    """
    model_id = model_name or "BAAI/bge-small-en-v1.5"
    try:
        model = _get_setfit_model(model_id)
        label_embeddings = _get_label_embeddings(model_id)
    except ImportError as exc:
        raise ImportError("setfit is required for torch classification. " "Install with: pip install setfit") from exc

    prompt_embedding = model.model_body.encode(
        [prompt],
        normalize_embeddings=True,
    )
    prompt_vector = np.asarray(prompt_embedding[0], dtype=np.float32)
    scores = label_embeddings @ prompt_vector
    best_index = int(np.argmax(scores))
    label = list(_CLASS_LABELS.keys())[best_index]
    score = float(scores[best_index])

    mapped_intent = _CLASS_TO_INTENT.get(label, Intent.conversation.name)
    next_step = _INTENT_TO_NEXT_STEP.get(mapped_intent, NextStep.respond_directly.name)

    return {
        "intent": mapped_intent,
        "needs_clarification": False,
        "missing_fields": [],
        "reasoning_summary": f"setfit-embed:{label} score={score:.4f}",
        "next_step": next_step,
    }
