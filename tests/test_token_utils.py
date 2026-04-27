"""Unit tests for shared.token_utils module-level helpers.

GIVEN text samples and model name hints
WHEN chars_per_token and estimate_text_tokens are called
THEN correct per-family ratios and ceiling-based token estimates are returned.
"""

import pytest

from shared.token_utils import chars_per_token, estimate_text_tokens


@pytest.mark.unit
@pytest.mark.parametrize(
    "model_name, expected_ratio",
    [
        ("llama3.2:8b", 3.5),
        ("llama2:70b", 3.5),
        ("LLAMA3:latest", 3.5),  # case-insensitive
        ("mistral:7b", 3.8),
        ("mistral-nemo:12b", 3.8),
        ("phi3:mini", 4.0),
        ("phi-2", 4.0),
        ("gemma2:9b", 4.0),  # unknown family → default
        ("unknown-model", 4.0),  # completely unknown
        ("", 4.0),  # empty string → default
    ],
)
def test_chars_per_token_family_ratios(model_name: str, expected_ratio: float) -> None:
    """GIVEN a model name WHEN chars_per_token is called THEN the correct family ratio is returned.

    Permutations:
    - llama family (case-insensitive) -> 3.5
    - mistral family -> 3.8
    - phi family -> 4.0
    - unknown model -> default 4.0
    - empty string -> default 4.0
    """
    assert chars_per_token(model_name) == expected_ratio


@pytest.mark.unit
@pytest.mark.parametrize(
    "text, ratio, expected",
    [
        ("", 4.0, 0),  # empty text → 0 tokens
        ("a" * 4, 4.0, 1),  # exactly 1 token at ratio 4.0
        ("a" * 5, 4.0, 2),  # ceiling: 5/4 = 1.25 → 2
        ("a" * 8, 4.0, 2),  # exactly 2 tokens
        ("a" * 9, 4.0, 3),  # ceiling: 9/4 = 2.25 → 3
        ("a" * 7, 3.5, 2),  # 7/3.5 = 2.0 exactly
        ("a" * 8, 3.5, 3),  # ceiling: 8/3.5 ≈ 2.29 → 3
        ("hello world", 4.0, 3),  # 11 chars / 4.0 = 2.75 → ceiling = 3
    ],
)
def test_estimate_text_tokens_ceiling(text: str, ratio: float, expected: int) -> None:
    """GIVEN text and ratio WHEN estimate_text_tokens is called THEN ceiling of chars/ratio is returned.

    Permutations:
    - empty text -> 0
    - exact multiple -> no ceiling effect
    - non-integer result -> ceiling applied
    - various ratios (3.5, 4.0)
    """
    assert estimate_text_tokens(text, ratio) == expected
