""""""

from __future__ import annotations

from dataclasses import asdict
from typing import Dict, List

from .tool_extractor import ToolExtractor
from .tool_spec import ToolSpec


def extract_tools_from_code(
    source: str, debug: bool = False, return_dicts: bool = True
):
    """
    Parse Python source with LibCST and extract a list of tool specs (dicts or ToolSpec objects) for
    top-level functions and class methods (non-nested).
    """
    collector = ToolExtractor()
    collector.debug = debug
    collector.from_code(source)
    if return_dicts:
        return [asdict(t) for t in collector.tools]
    else:
        return collector.tools


def extract_tools_from_file(path: str, debug: bool = False, return_dicts: bool = True):
    """
    Docstring for extract_tools_from_file

    :param path: Description
    :type path: str
    :param return_dicts: If True, return list of dicts; if False, return list of ToolSpec objects
    :type return_dicts: bool
    :return: List of tool specs (dicts or ToolSpec objects)
    :rtype: List[Dict] or List[ToolSpec]
    """
    with open(path, "r", encoding="utf8") as f:
        source = f.read()
    return extract_tools_from_code(source, debug, return_dicts=return_dicts)


# Optional: format for OpenAI "tools" (function calling) style
def to_openai_tools(tools: List[ToolSpec]) -> List[Dict]:
    """
    Docstring for to_openai_tools

    :param tools: Description
    :type tools: List[Dict]
    :return: Description
    :rtype: List[Dict]
    """
    out = []
    for t in tools:
        out.append(
            {
                "type": "function",
                "function": {
                    "name": t.qualified_name.replace(
                        ".", "__"
                    ),  # flatten for API constraints
                    "description": t.description or (t.docstring or "")[:300],
                    "parameters": t.parameters_schema,
                },
            }
        )
    return out
