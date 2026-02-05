# Prompt management for Agentix CLI

import glob
import json
import sys

from ..agentix_config import AgentixConfig
from ..constants import SYSTEM_PROMPTS_DIR
from ..file_utils import get_file
from ..tools import ast_tools, cst_tools
from ..tools.describe_tools import extract_tools_from_file, to_openai_tools


def get_system_prompt(args: AgentixConfig) -> str:
    """Load system prompts from files and return formatted."""
    systemprompt = ""
    # get the system prompts and map their paths to their friendly names (no dir, no ext)
    prompts = {
        p.replace(SYSTEM_PROMPTS_DIR, "").split(".")[0]: p
        for p in glob.glob(f"{SYSTEM_PROMPTS_DIR}*.*")
    }
    if args.debug:
        print(
            f"Available system prompts: {json.dumps(prompts, indent=2)}",
            file=sys.stderr,
        )
    for canned_system_prompt_path in args.system or []:
        prompt_path = prompts[canned_system_prompt_path]
        if args.debug:
            print(f"Loading system prompt from: {prompt_path}", file=sys.stderr)
        systemprompt += get_file(prompt_path)
    return f"[SYSTEM]\n{systemprompt}\n[END SYSTEM]\n\n"


def get_user_prompt(args: AgentixConfig) -> str:
    """Assemble user prompt from CLI arguments."""
    return "\n".join(args.user or [])


def get_tools_prompt(args: AgentixConfig) -> str:
    """Assemble tools prompt from CLI arguments."""
    f = ""
    tools = []
    for t in args.tools or []:
        if args.debug:
            print(f"Processing tool: {t}", file=sys.stderr)
        match t:
            case "cst":
                f = cst_tools.__file__
            case "ast":
                f = ast_tools.__file__
            case _:
                if args.debug:
                    print(f"Unknown tool: {t}", file=sys.stderr)
                f = ""
        if f:
            tool_data = {}
            try:
                tool_data = extract_tools_from_file(
                    f, debug=args.debug, return_dicts=False
                )
                tools.append(to_openai_tools(tool_data))
            except Exception as e:
                print("tool_data:", tool_data, file=sys.stderr)
                print(f"Error extracting tools from {f}: {e}", file=sys.stderr)

    return f"[TOOLS]\n{json.dumps(tools, indent=2)}\n[END TOOLS]\n\n"


def get_prompts(args: AgentixConfig) -> dict:
    """List available system prompts with preview lines."""
    prompts = {}
    for prompt_glob in [glob.glob(f"{SYSTEM_PROMPTS_DIR}*.*")]:
        if args.debug:
            print(f"Prompt: {prompt_glob}", file=sys.stderr)
        if prompt_glob and isinstance(prompt_glob, list):
            for prompt in prompt_glob:
                with open(prompt, "r", encoding="utf8") as f:
                    lines = [l for l in f.readlines() if l != "\n" and l != ""]
                first_lines = lines[:2]
                prompts[prompt.replace(SYSTEM_PROMPTS_DIR, "").split(".")[0]] = (
                    first_lines
                )
    return prompts
