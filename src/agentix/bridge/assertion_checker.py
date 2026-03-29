"""Assertion extraction and mechanical verification for task-node syntheses.

Called from ``_run_task_node`` after a synthesis text is produced to extract
structured verifiable claims and then check them against the local file system.

Assertion pipeline
------------------
1. ``extract_assertions(synthesis_text, llm_iter_fn)``
   Issues a cheap LLM call (no tools, constrained JSON response) to extract
   zero or more ``AssertionRecord`` objects from the synthesis text.

2. ``verify_assertion(assertion, codebase_root)``
   Mechanically verifies the assertion against the file system in-process.
   Mutates ``assertion.verified`` / ``assertion.error`` in-place.

Verification types
------------------
``exists``  check is a (relative or absolute) file/directory path.
            Verified with ``os.path.exists``.

``value``   check is ``"path::substring"`` — the exact substring must appear
            in the named file.

``regex``   check is ``"path::pattern"`` — the regex pattern must match at least
            one location in the named file.

``count``   check is a plain integer string N.  Cannot be verified mechanically
            without knowing *what* to count, so ``verified`` is set to ``None``.

Any unknown type also receives ``verified=None`` so the assertion is recorded but
does not trigger a re-synthesis.
"""

from __future__ import annotations

import json
import logging
import os
import re
from typing import Callable, Iterator

from shared.models.response import ChunkType, ResponseChunk
from shared.models.task_node import AssertionRecord

logger = logging.getLogger("agentix.assertion_checker")

# ---------------------------------------------------------------------------
# LLM prompt for assertion extraction
# ---------------------------------------------------------------------------

_EXTRACT_SYSTEM_PROMPT = """\
You are a fact extractor. Your only job is to extract every specific, \
verifiable claim from the synthesis text provided by the user.

Return a JSON array of objects — ONLY the JSON array, no other text:
[{"fact": "<short description>", "type": "<exists|value|regex|count>", "check": "<expression>"}]

Type guide:
- "exists"  → check is a file or directory path (relative to project root or absolute)
- "value"   → check is "relative/path::exact substring" — the substring must appear in that file
- "regex"   → check is "relative/path::regex pattern" — the pattern must match in that file
- "count"   → check is an integer string N; the claim involves at least N items (not verified automatically)

Rules:
- Extract only claims with a concrete, checkable expression.
- Skip vague or subjective claims.
- Return an empty array [] if there are no verifiable claims.
- Output ONLY the JSON array with no surrounding text or markdown fences."""

_EXTRACT_USER_TEMPLATE = "Synthesis:\n{synthesis}"


# ---------------------------------------------------------------------------
# Public API
# ---------------------------------------------------------------------------


def extract_assertions(
    synthesis_text: str,
    llm_iter_fn: Callable[..., Iterator[ResponseChunk]],
    max_assertions: int = 10,
) -> list[AssertionRecord]:
    """Extract verifiable assertions from a synthesis string via an LLM call.

    The LLM is asked to return a JSON array of ``{"fact", "type", "check"}``
    objects using a constrained system prompt.  Gracefully returns an empty
    list on any failure so callers never crash.

    Args:
        synthesis_text: Text produced by a completed task node.
        llm_iter_fn: Callable matching ``bridge._iter_llm_chunks`` signature.
                     Invoked without tools (``tools=None``).
        max_assertions: Upper bound on assertions kept (extra ones are dropped).

    Returns:
        ``AssertionRecord`` list with ``verified=None`` on each item (not yet
        checked).  Empty list on error or empty synthesis.
    """
    if not synthesis_text or synthesis_text.strip() in ("", "(no synthesis produced)"):
        return []

    messages = [
        {"role": "system", "content": _EXTRACT_SYSTEM_PROMPT},
        {"role": "user", "content": _EXTRACT_USER_TEMPLATE.format(synthesis=synthesis_text)},
    ]

    raw = ""
    try:
        for chunk in llm_iter_fn(messages, None):  # no tools
            if chunk.type == ChunkType.CONTENT:
                raw += chunk.content
    except Exception as exc:
        logger.warning("Assertion extraction LLM call failed: %s", exc)
        return []

    raw = raw.strip()
    try:
        data = _parse_json_list(raw)
    except (ValueError, json.JSONDecodeError) as exc:
        logger.debug("Assertion JSON parse failed: %s | raw=%.200s", exc, raw)
        return []

    records: list[AssertionRecord] = []
    for item in data[:max_assertions]:
        try:
            records.append(
                AssertionRecord(
                    fact=str(item.get("fact", "")),
                    type=str(item.get("type", "exists")),
                    check=str(item["check"]) if item.get("check") else None,
                )
            )
        except Exception:
            continue

    return records


def verify_assertion(assertion: AssertionRecord, codebase_root: str = "") -> AssertionRecord:
    """Mechanically verify one assertion against the local file system.

    Mutates ``assertion.verified`` and ``assertion.error`` in-place and returns
    the same object for chaining.

    Supported types
    ---------------
    ``exists``  — ``os.path.exists(resolved_path)``
    ``value``   — substring presence; check format: ``relative/path::substring``
    ``regex``   — regex match; check format: ``relative/path::pattern``
    ``count``   — not verified (leaves ``verified=None``)

    Args:
        assertion: The ``AssertionRecord`` to verify (mutated in-place).
        codebase_root: Project root used for resolving relative paths.
                       Defaults to the process working directory.

    Returns:
        The mutated ``AssertionRecord``.
    """
    root = codebase_root or os.getcwd()
    check = (assertion.check or "").strip()
    atype = (assertion.type or "").lower()

    try:
        if atype == "exists":
            path = check if os.path.isabs(check) else os.path.join(root, check)
            assertion.verified = os.path.exists(path)
            if not assertion.verified:
                assertion.error = f"Path not found: {path}"

        elif atype in ("value", "regex"):
            if "::" not in check:
                # No path separator — cannot resolve without a target file
                assertion.verified = None
                return assertion

            path_part, pattern = check.split("::", 1)
            path = path_part if os.path.isabs(path_part) else os.path.join(root, path_part)

            if not os.path.exists(path):
                assertion.verified = False
                assertion.error = f"File not found: {path}"
            else:
                with open(path, "r", encoding="utf-8", errors="replace") as fh:
                    content = fh.read()

                if atype == "value":
                    assertion.verified = pattern in content
                    if not assertion.verified:
                        assertion.error = f"Substring not found: {pattern!r}"
                else:
                    try:
                        assertion.verified = bool(re.search(pattern, content, re.MULTILINE))
                        if not assertion.verified:
                            assertion.error = f"Pattern not matched: {pattern!r}"
                    except re.error as exc:
                        assertion.verified = False
                        assertion.error = f"Invalid regex ({exc}): {pattern!r}"

        elif atype == "count":
            # Count assertions require dynamic context — skip mechanical check
            assertion.verified = None

        else:
            assertion.verified = None

    except OSError as exc:
        assertion.verified = False
        assertion.error = str(exc)

    return assertion


# ---------------------------------------------------------------------------
# Internal helpers
# ---------------------------------------------------------------------------


def _parse_json_list(raw: str) -> list[dict]:
    """Extract a JSON array from an LLM response string.

    Tries a direct parse first; falls back to finding the first ``[…]`` block.

    Args:
        raw: String that may contain a JSON array, possibly wrapped in prose.

    Returns:
        Parsed list of dicts.

    Raises:
        ValueError: If no JSON array can be found or parsed.
    """
    # Try direct parse
    try:
        result = json.loads(raw)
        if isinstance(result, list):
            return result
    except json.JSONDecodeError:
        pass

    # Find first [ … ] block
    start = raw.find("[")
    end = raw.rfind("]")
    if start != -1 and end > start:
        candidate = raw[start : end + 1]
        try:
            result = json.loads(candidate)
            if isinstance(result, list):
                return result
        except json.JSONDecodeError:
            pass

    raise ValueError(f"No JSON array found in text (length={len(raw)})")
