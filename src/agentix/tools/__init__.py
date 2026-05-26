"""agentix.tools package initializer"""

try:
    from . import ast_tools, cst_tools, describe_tools
    from .describe_tools import (
        ToolExtractor,
        extract_tools_from_code,
        extract_tools_from_file,
        to_openai_tools,
    )

    TOOLS_AVAILABLE = True
except ImportError as e:
    # Tools not available (missing libcst or other dependencies)
    TOOLS_AVAILABLE = False
    ast_tools = None
    cst_tools = None
    describe_tools = None


def extract_cst_tools():
    if not TOOLS_AVAILABLE or cst_tools is None:
        return []
    return extract_tools_from_file(cst_tools.__file__, return_dicts=False)


__all__ = [
    "ast_tools",
    "cst_tools",
    "ToolExtractor",
    "to_openai_tools",
    "describe_tools",
    "extract_tools_from_file",
    "extract_tools_from_code",
    "extract_cst_tools",
]
