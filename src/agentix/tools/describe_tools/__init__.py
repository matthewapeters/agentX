"""
Docstring for agentix.tools.describe_tools
"""

import json

from .tool_extractor import ToolExtractor
from .tool_spec import ToolSpec
from .tools import extract_tools_from_code, extract_tools_from_file, to_openai_tools

__all__ = [
    "ToolExtractor",
    "to_openai_tools",
    "extract_tools_from_file",
    "extract_tools_from_code",
    "ToolSpec",
]


if __name__ == "__main__":
    import sys

    file = sys.argv[1] if len(sys.argv) > 1 else "cst_tools.py"
    tools = extract_tools_from_file(file)
    print(json.dumps(tools, indent=2, ensure_ascii=False))
    # To OpenAI-style:
    print(json.dumps(to_openai_tools(tools), indent=2, ensure_ascii=False))
