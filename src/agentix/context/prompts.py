# Prompt management for Agentix CLI

import json
import logging

from ..agentix_config import AgentixConfig
from ..prompt_loader import PromptLoader

logger = logging.getLogger(__name__)


def get_system_prompt(args: AgentixConfig) -> str:
    """Load system prompts from files and return formatted."""
    loader = PromptLoader(getattr(args, "system_prompts_dir", None))
    return loader.get_formatted_system_prompt(args.system or [], debug=args.debug)


def get_user_prompt(args: AgentixConfig) -> str:
    """Assemble user prompt from CLI arguments."""
    return "\n".join(args.user or [])


def get_tools_prompt(args: AgentixConfig) -> str:
    """Assemble tools prompt from CLI arguments."""
    # Lazy import to avoid circular dependencies and missing libcst
    from ..tools import ast_tools, cst_tools
    from ..tools.describe_tools import extract_tools_from_file, to_openai_tools

    f = ""
    tools = []
    for t in args.tools or []:
        if args.debug:
            logger.debug("Processing tool: %s", t)
        match t:
            case "cst":
                if cst_tools is not None:
                    f = cst_tools.__file__
            case "ast":
                if ast_tools is not None:
                    f = ast_tools.__file__
            case _:
                if args.debug:
                    logger.debug("Unknown tool: %s", t)
                f = ""
        if f:
            tool_data = {}
            try:
                tool_data = extract_tools_from_file(f, debug=args.debug, return_dicts=False)
                tools.append(to_openai_tools(tool_data))
            except Exception as e:
                logger.error("tool_data: %s — Error extracting tools from %s: %s", tool_data, f, e)

    return f"[TOOLS]\n{json.dumps(tools, indent=2)}\n[END TOOLS]\n\n"


def get_prompts(args: AgentixConfig) -> dict:
    """List available system prompts with preview lines."""
    loader = PromptLoader(getattr(args, "system_prompts_dir", None))
    return loader.preview()
