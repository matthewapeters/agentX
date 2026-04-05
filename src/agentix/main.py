"""agentix main module"""

import json
import logging

from .agent import agentix
from .agentix_config import AgentixConfig
from .constants import SESSIONS_METADATA_FILE
from .context.prompts import get_prompts
from .models import get_models
from .logging_config import configure_logging

# Configure logging at module import
configure_logging()

logger = logging.getLogger(__name__)

try:
    import agentix.server as _server_mod  # noqa: F401 – check availability only

    SERVER_AVAILABLE = True
except ImportError:
    SERVER_AVAILABLE = False


def main(args: AgentixConfig) -> None:
    """agentix main functionality"""
    match args.action:
        case "list_models":
            print(json.dumps(get_models(args), indent=2))
            return
        case "list_prompts":
            print(json.dumps(get_prompts(args), indent=2))
            return
        case "list_sessions":
            try:
                with open(SESSIONS_METADATA_FILE, "r", encoding="utf-8") as f:
                    for line in f.readlines():
                        print(line.strip())
            except FileNotFoundError:
                logger.warning("No sessions found")
            return
        case "classify":
            from .bridge.classify_prompt import classify_prompt
            from shared.models.context import Context

            prompt = " ".join(args.user or [])
            if not prompt:
                logger.error("--classify requires --user 'prompt text'")
                return

            context = Context()

            print("=" * 80)
            print("PROMPT:")
            print(prompt)
            print("=" * 80)
            print()

            try:
                result = classify_prompt(
                    args,
                    prompt,
                    context,
                    history=[],
                    max_tokens=500,
                )

                print("CLASSIFICATION RESULT:")
                print(
                    json.dumps(
                        {
                            "intent": result.intent.name,
                            "next_step": result.next_step.name,
                            "needs_clarification": result.needs_clarification,
                            "missing_fields": result.missing_fields,
                            "reasoning_summary": result.reasoning_summary,
                        },
                        indent=2,
                    )
                )
            except Exception as e:
                logger.exception("Error during classification: %s", e)
            return
        case "serve":
            if not SERVER_AVAILABLE:
                logger.error("Server functionality requires fastapi. Install with: uv pip install fastapi uvicorn")
                return
            from .server import start_server as _start_server

            _start_server(args.port)
            return
        case "run_agentix":
            agentix(args)
            return
        case _:
            logger.error("Unknown action: %s", args.action)
            return


if __name__ == "__main__":
    # Get CLI config
    cli_config = AgentixConfig.cli_arguments()
    # Convert dataclass to dict for merging
    base_config_dict = cli_config.__dict__
    # Load local config from .toml
    local_config = AgentixConfig.load_local_config()
    # Merge configs (local overrides CLI/defaults)
    merged_config_dict = AgentixConfig.merge_configs(base_config_dict, local_config)
    # Reconstruct AgentixConfig from merged dict
    merged_config = AgentixConfig(**merged_config_dict)
    main(merged_config)
