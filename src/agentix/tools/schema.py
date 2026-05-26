"""
Runtime function-to-JSON-schema conversion for agentix tool registration.

Inspired by the MIT-licensed lmstudio-python SDK pattern of deriving tool
schemas directly from Python callable signatures and docstrings, with no
external dependencies beyond stdlib.
"""

from __future__ import annotations

import inspect
import typing
from typing import Any, Callable, Optional, get_type_hints


class SchemaGenerationError(Exception):
    """Raised when a tool schema cannot be derived from a callable."""


# Mapping from Python built-in types to JSON Schema type strings.
_PY_TO_JSON: dict[type, str] = {
    str: "string",
    int: "integer",
    float: "number",
    bool: "boolean",
    list: "array",
    dict: "object",
    bytes: "string",
}


def _resolve_json_type(annotation: Any) -> dict:
    """
    Convert a single Python type annotation to a JSON Schema fragment.

    Handles:
    - Primitives: str, int, float, bool, list, dict
    - Optional[T] → unwrapped T (caller marks as non-required)
    - list[T] → {"type": "array", "items": <T schema>}
    - dict[str, V] → {"type": "object"}
    - Union / complex types → degraded to {"type": "string"} with a note
    """
    origin = typing.get_origin(annotation)
    args = typing.get_args(annotation)

    # Optional[T] is Union[T, None]
    if origin is typing.Union:
        non_none = [a for a in args if a is not type(None)]
        if len(non_none) == 1:
            return _resolve_json_type(non_none[0])
        # Multi-union — degrade gracefully
        return {"type": "string", "description": "(complex union type — represented as string)"}

    # list[T]
    if origin is list:
        schema: dict[str, Any] = {"type": "array"}
        if args:
            schema["items"] = _resolve_json_type(args[0])
        return schema

    # dict[K, V]
    if origin is dict:
        return {"type": "object"}

    # Plain primitive
    if annotation in _PY_TO_JSON:
        return {"type": _PY_TO_JSON[annotation]}

    # Inspect.Parameter.empty or bare Any
    if annotation is inspect.Parameter.empty or annotation is Any:
        return {"type": "string"}

    # Fallback for anything else (custom classes, TypeVars, etc.)
    return {
        "type": "string",
        "description": f"(type {getattr(annotation, '__name__', str(annotation))} represented as string)",
    }


def _is_optional(annotation: Any) -> bool:
    """Return True if the annotation is Optional[T] (i.e. Union[T, None])."""
    return typing.get_origin(annotation) is typing.Union and type(None) in typing.get_args(annotation)


def extract_tool_schema(fn: Callable) -> dict:
    """
    Derive an OpenAI-compatible tool schema from a Python callable.

    The schema is suitable for inclusion in an Ollama or OpenAI ``tools``
    array::

        {
            "type": "function",
            "function": {
                "name": "multiply",
                "description": "Multiply two numbers.",
                "parameters": {
                    "type": "object",
                    "properties": {
                        "a": {"type": "number"},
                        "b": {"type": "number"}
                    },
                    "required": ["a", "b"]
                }
            }
        }

    Args:
        fn: A Python callable with a docstring and type-annotated parameters.

    Returns:
        OpenAI-format tool schema dict.

    Raises:
        SchemaGenerationError: If the callable has no docstring.
    """
    if not fn.__doc__ or not fn.__doc__.strip():
        raise SchemaGenerationError(
            f"Tool function '{fn.__name__}' must have a docstring describing what it does. "
            "The LLM uses the docstring to decide when to call the tool."
        )

    description = inspect.cleandoc(fn.__doc__)

    # Resolve type hints (handles forward references)
    try:
        hints = get_type_hints(fn)
    except Exception:
        hints = {}
    hints.pop("return", None)

    sig = inspect.signature(fn)
    properties: dict[str, dict] = {}
    required: list[str] = []

    for param_name, param in sig.parameters.items():
        if param_name == "self":
            continue

        annotation = hints.get(param_name, inspect.Parameter.empty)
        prop_schema = _resolve_json_type(annotation)

        # Absorb any inline description from Annotated[T, "description"] if present
        if typing.get_origin(annotation) is typing.Annotated:
            inner_args = typing.get_args(annotation)
            prop_schema = _resolve_json_type(inner_args[0])
            for meta in inner_args[1:]:
                if isinstance(meta, str):
                    prop_schema["description"] = meta

        properties[param_name] = prop_schema

        # A parameter is required if it has no default and is not Optional
        has_default = param.default is not inspect.Parameter.empty
        is_opt = _is_optional(annotation)
        if not has_default and not is_opt:
            required.append(param_name)

    parameters_schema: dict[str, Any] = {
        "type": "object",
        "properties": properties,
    }
    if required:
        parameters_schema["required"] = required

    return {
        "type": "function",
        "function": {
            "name": fn.__name__,
            "description": description,
            "parameters": parameters_schema,
        },
    }
