"""Utility functions for extracting docstrings from functions."""

import ast as pyast
from typing import Optional

import libcst as cst


def _extract_docstring_from_function(fn: cst.FunctionDef) -> Optional[str]:
    """
    Return the function docstring if present (entire content, including multi-line).
    """
    if not isinstance(fn.body, cst.IndentedBlock) or not fn.body.body:
        return None
    first = fn.body.body[0]
    if (
        isinstance(first, cst.SimpleStatementLine)
        and len(first.body) == 1
        and isinstance(first.body[0], cst.Expr)
        and isinstance(first.body[0].value, cst.SimpleString)
    ):
        raw = first.body[0].value  # includes quotes
        raw_value = raw.value
        try:
            # Use ast.literal_eval to safely unescape multi-line string literals
            unescaped_docstring = pyast.literal_eval(raw_value)
            return unescaped_docstring.strip()
        except Exception:
            # Fallback: manually process multi-line strings
            lines = raw_value.strip("'\"").splitlines()
            processed_lines = [line.strip() for line in lines if line.strip()]
            print(f"Processed lines: {processed_lines}")  # Debug log
            return "\n".join(processed_lines)
    return None


def _docstring_summary(doc: Optional[str]) -> Optional[str]:
    if not doc:
        return None
    # summary = first non-empty line
    for line in doc.strip().splitlines():
        s = line.strip()
        if s:
            return s
    return None


def _extract_docstring_from_function(fn: cst.FunctionDef) -> Optional[str]:
    """
    Return the function docstring if present (entire content, including multi-line).
    """
    if not isinstance(fn.body, cst.IndentedBlock) or not fn.body.body:
        return None
    first = fn.body.body[0]
    if (
        isinstance(first, cst.SimpleStatementLine)
        and len(first.body) == 1
        and isinstance(first.body[0], cst.Expr)
        and isinstance(first.body[0].value, cst.SimpleString)
    ):
        raw = first.body[0].value.value  # includes quotes
        # print(f"Raw docstring: {raw}")  # Debug log
        try:
            # Use ast.literal_eval to safely unescape multi-line string literals
            # Ensure all lines are preserved and unescaped
            unescaped_docstring = pyast.literal_eval(raw)
            return unescaped_docstring.strip()
        except Exception as e:
            print(f"Error parsing docstring: {e}")  # Debug log
            # Fallback: return raw string with stripped quotes
            return raw.strip("'\"")
    return None


def _docstring_summary(doc: Optional[str]) -> Optional[str]:
    if not doc:
        return None
    # summary = first non-empty line
    for line in doc.strip().splitlines():
        s = line.strip()
        if s:
            return s
    return None
