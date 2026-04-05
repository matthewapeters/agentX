# Prompt management for Agentix CLI

import glob
import json
import logging
import sys

from ..agentix_config import AgentixConfig
from ..constants import SYSTEM_PROMPTS_DIR
from ..file_utils import get_file

logger = logging.getLogger(__name__)


def get_system_prompt(args: AgentixConfig) -> str:
    """Load system prompts from files and return formatted."""
    systemprompt = ""
    # get the system prompts and map their paths to their friendly names (no dir, no ext)
    prompts = {p.replace(SYSTEM_PROMPTS_DIR, "").split(".")[0]: p for p in glob.glob(f"{SYSTEM_PROMPTS_DIR}*.*")}
    if args.debug:
        logger.debug("Available system prompts: %s", json.dumps(prompts, indent=2))
    for canned_system_prompt_path in args.system or []:
        prompt_path = prompts[canned_system_prompt_path]
        if args.debug:
            logger.debug("Loading system prompt from: %s", prompt_path)
        systemprompt += get_file(prompt_path)
    return f"[SYSTEM]\n{systemprompt}\n[END SYSTEM]\n\n"


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
    prompts = {}
    for prompt_glob in [glob.glob(f"{SYSTEM_PROMPTS_DIR}*.*")]:
        if args.debug:
            logger.debug("Prompt: %s", prompt_glob)
        if prompt_glob and isinstance(prompt_glob, list):
            for prompt in prompt_glob:
                with open(prompt, "r", encoding="utf8") as f:
                    lines = [l for l in f.readlines() if l != "\n" and l != ""]
                first_lines = lines[:2]
                prompts[prompt.replace(SYSTEM_PROMPTS_DIR, "").split(".")[0]] = first_lines
    return prompts
