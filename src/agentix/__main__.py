"""Entry point for running agentix as a module."""

from .agentix_config import AgentixConfig
from .main import main

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
