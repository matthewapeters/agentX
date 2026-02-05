"""
Docstring for agentix.agentix_config
"""

import argparse
import os
from argparse import Namespace
from dataclasses import dataclass

import tomli

from .constants import DEFAULT_SESSION_ID, DEFAULT_TEMPERATURE

# pylint: disable=too-many-instance-attributes


@dataclass
class AgentixConfig:
    """Configuration settings for Agentix"""

    debug: bool = False
    list_models: bool = False
    list_sessions: bool = False
    list_prompts: bool = False
    session: str = DEFAULT_SESSION_ID
    system: list[str] | None = None
    model: str | None = None
    temperature: float = 0.7
    user: list[str] | None = None
    file_path: list[str] | None = None
    replace_file: bool = False
    serve: bool = False
    port: int = 8000
    with_frontend: bool = False
    tools: list[str] | None = None

    @property
    def action(self) -> str:
        """
        Docstring for action

        :param self: Description
        :return: Description
        :rtype: str
        """
        if self.list_models:
            return "list_models"
        if self.list_sessions:
            return "list_sessions"
        if self.list_prompts:
            return "list_prompts"
        if self.serve:
            return "serve"
        return "run_agentix"

    @staticmethod
    def cli_arguments() -> "AgentixConfig":
        """
        Docstring for cli_arguments

        :return: Description
        :rtype: AgentixConfig
        """
        args = argparse.ArgumentParser(description="Agentix CLI")
        args.add_argument(
            "--list-models",
            dest="list_models",
            default=False,
            action="store_true",
            help="List all available models",
        )
        args.add_argument(
            "--list-sessions",
            dest="list_sessions",
            default=False,
            action="store_true",
            help="List all sessions",
        )
        args.add_argument(
            "--list-prompts",
            dest="list_prompts",
            default=False,
            action="store_true",
            help="List all system prompts",
        )
        args.add_argument(
            "--session",
            type=str,
            dest="session",
            default=DEFAULT_SESSION_ID,
            help="Session ID for the conversation",
        )
        args.add_argument(
            "--system",
            type=str,
            action="append",
            dest="system",
            help="The system prompt to send to the API",
        )
        args.add_argument("--model", type=str, dest="model", help="The model to use")
        args.add_argument(
            "--temp",
            "--temperature",
            type=float,
            dest="temperature",
            default=DEFAULT_TEMPERATURE,
            help="Sampling temperature",
        )
        args.add_argument(
            "--user",
            type=str,
            action="append",
            dest="user",
            help="The user prompt to send to the API",
        )
        args.add_argument(
            "--file",
            type=str,
            action="append",
            dest="file_path",
            help="Path to the file containing the prompt",
        )
        args.add_argument(
            "--replace-file",
            dest="replace_file",
            default=False,
            action="store_true",
            help="Replace the file contents with the LLM output",
        )
        args.add_argument(
            "--debug", type=bool, default=False, help="Enable debug output"
        )
        args.add_argument(
            "--with-front-end",
            dest="with_front_end",
            default=False,
            action="store_true",
            help="Agentix is working with a front-end",
        )
        args.add_argument(
            "--serve",
            dest="serve",
            default=False,
            action="store_true",
            help="Launch FastAPI server",
        )
        args.add_argument(
            "--port",
            type=int,
            dest="port",
            default=8000,
            help="Port to serve on (default: 8000)",
        )
        args.add_argument(
            "--tools",
            type=str,
            action="append",
            default=["cst"],
            dest="tools",
            help="Specify tools to use",
        )
        args: Namespace = args.parse_args()

        return AgentixConfig(
            list_models=args.list_models,
            list_sessions=args.list_sessions,
            list_prompts=args.list_prompts,
            session=args.session,
            system=args.system,
            model=args.model,
            temperature=args.temperature,
            user=args.user,
            file_path=args.file,
            replace_file=args.replace_file,
            serve=args.serve,
            port=args.port,
            with_frontend=args.with_frontend,
            tools=args.tools,
            debug=args.debug,
        )

    # Helper functions for config discovery and merging
    @staticmethod
    def find_local_config(filename="agentix_config.toml") -> str | None:
        """
        Search for a local .toml config file in the current working directory.
        Returns the path if found, else None.
        """
        cwd = os.getcwd()
        local_path = os.path.join(cwd, filename)
        if os.path.isfile(local_path):
            return local_path
        return None

    @staticmethod
    def load_local_config(filename="agentix_config.toml"):
        """
        Loads and parses a local .toml config file if present.
        Returns a dict of config values, or empty dict if not found.
        """
        path = AgentixConfig.find_local_config(filename)
        if path:
            with open(path, "rb") as f:
                return tomli.load(f)
        return {}

    @staticmethod
    def merge_configs(base_config, override_config):
        """
        Merge override_config into base_config, overriding values.
        """
        merged = base_config.copy()
        for k, v in override_config.items():
            if isinstance(v, dict) and k in merged and isinstance(merged[k], dict):
                merged[k] = AgentixConfig.merge_configs(merged[k], v)
            else:
                merged[k] = v
        return merged
