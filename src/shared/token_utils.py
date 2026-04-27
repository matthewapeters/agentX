"""Token-count utility functions (TOK-02 strategy).

These are pure functions with no dependency on context state and can be
imported by any layer without introducing circular dependencies.

The TOK-02 strategy uses model-family character-per-token ratios that are
meaningfully more accurate than the flat ``len(text) // 4`` baseline while
adding zero latency and requiring no extra dependencies.
"""

import math

# TOK-02 char/token ratio table keyed by lower-cased model name prefix.
# The default applies when no prefix matches.
_FAMILY_RATIOS: list[tuple[str, float]] = [
    ("llama", 3.5),
    ("mistral", 3.8),
    ("phi", 4.0),
]
_DEFAULT_RATIO: float = 4.0


def chars_per_token(model_name: str) -> float:
    """Return the TOK-02 char/token ratio for a model family.

    Args:
        model_name: Model identifier string (e.g. ``"llama3.2"``).

    Returns:
        Characters-per-token ratio as a float.  Higher values indicate
        more characters are packed into a single token.
    """
    lowered = (model_name or "").strip().lower()
    for prefix, ratio in _FAMILY_RATIOS:
        if lowered.startswith(prefix):
            return ratio
    return _DEFAULT_RATIO


def estimate_text_tokens(text: str, ratio: float) -> int:
    """Estimate the token count for ``text`` using a character-per-token ratio.

    Args:
        text: Input text to estimate.
        ratio: Characters-per-token ratio (see :func:`chars_per_token`).

    Returns:
        Estimated token count, always >= 0.  Returns 0 for empty ``text``.
    """
    if not text:
        return 0
    return int(math.ceil(len(text) / ratio))
