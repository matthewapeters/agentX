"""
Docstring for agentix.agentix_config
"""

import argparse
import os
from argparse import Namespace
from dataclasses import dataclass

try:
    import tomllib  # Python 3.11+
except ImportError:
    try:
        import tomli as tomllib  # Fallback for Python < 3.11
    except ImportError:
        tomllib = None  # TOML support disabled

from .constants import DEFAULT_SESSION_ID, DEFAULT_TEMPERATURE

# pylint: disable=too-many-instance-attributes


@dataclass
class AgentixConfig:
    """Configuration settings for Agentix"""

    debug: bool = False
    list_models: bool = False
    list_sessions: bool = False
    list_prompts: bool = False
    classify: bool = False
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
    ollama_host: str = "localhost:11434"
    classify_prompts: bool = True  # Enable prompt classification by default
    classification_model: str | None = None
    classification_max_tokens: int | None = None
    classification_backend: str = "ollama"
    classification_torch_model: str | None = None
    classification_torch_device: int | None = None
    response_format: str | None = None  # For Ollama: "json" enforces JSON-only output
    max_tool_rounds: int = 10  # Maximum tool-call rounds per agent loop
    max_task_depth: int = 10  # Maximum recursive task-node depth
    max_synthesis_retries: int = 3  # Synthesis retry attempts for task nodes

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
        if self.classify:
            return "classify"
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
            "--classify",
            dest="classify",
            default=False,
            action="store_true",
            help="Test classification for a prompt (use with --user)",
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
        args.add_argument("--debug", type=bool, default=False, help="Enable debug output")
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
        args.add_argument(
            "--classification-backend",
            type=str,
            default="ollama",
            dest="classification_backend",
            help="Classification backend: ollama or torch",
        )
        args.add_argument(
            "--classification-torch-model",
            type=str,
            default=None,
            dest="classification_torch_model",
            help="Hugging Face model id for torch classification",
        )
        args.add_argument(
            "--classification-torch-device",
            type=int,
            default=None,
            dest="classification_torch_device",
            help="Transformers device index (-1 for CPU, 0 for first GPU)",
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
            file_path=args.file_path,
            replace_file=args.replace_file,
            serve=args.serve,
            port=args.port,
            with_frontend=args.with_front_end,
            tools=args.tools,
            debug=args.debug,
            classification_backend=args.classification_backend,
            classification_torch_model=args.classification_torch_model,
            classification_torch_device=args.classification_torch_device,
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
        if tomllib is None:
            return {}  # TOML support not available

        path = AgentixConfig.find_local_config(filename)
        if path:
            with open(path, "rb") as f:
                return tomllib.load(f)
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
