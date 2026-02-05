"""agentix main module"""

import json
import sys

from .agent import agentix
from .agentix_config import AgentixConfig
from .constants import SESSIONS_METADATA_FILE
from .context.prompts import get_prompts
from .models import get_models
from .server import start_server


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
                print("No sessions found", file=sys.stderr)
            return
        case "serve":
            start_server(args.port)
            return
        case "run_agentix":
            agentix(args)
            return
        case _:
            print(f"Unknown action: {args.action}", file=sys.stderr)
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
