# Model management for Agentix CLI

import json
import logging
import sys

import requests

from .constants import OLLAMA_API_BASE, OLLAMA_MODELS_ENDPOINT

logger = logging.getLogger(__name__)


def get_models(args, filter_by_model=True):
    """
    Fetch available models from Ollama API.

    Args:
        args: AgentixConfig with model and ollama_host settings
        filter_by_model: If True, filter to models matching args.model; if False, return all models
    """
    # Use configured host or fallback to constant
    ollama_base = f"http://{args.ollama_host}" if hasattr(args, "ollama_host") and args.ollama_host else OLLAMA_API_BASE

    result = requests.get(f"{ollama_base}{OLLAMA_MODELS_ENDPOINT}")
    models_json = result.json()

    if args.debug:
        logger.debug("Available models: %s", json.dumps(models_json, indent=2))
        if filter_by_model:
            logger.debug("Filtering models with prefix: %s", args.model)

    # Only filter if requested and model is configured
    if filter_by_model and args.model:
        models = [
            m
            for m in models_json["models"]
            # filter based on model_name if provided
            if m["name"].startswith(args.model)
        ]
        # return the first matching model or default to the first model
        if models and len(models) == 1:
            return models
        return models
    else:
        # Return all models
        return models_json["models"]
    if models and len(models) == 1:
        return models
    return models_json["models"]


def parse_parameter_size(param_size: str) -> int:
    """
    Convert a parameter size string (e.g., "1.5B") to an integer value.
    Supported suffixes: K (thousand), M (million), B (billion).
    """
    suffix_multipliers = {
        "K": 1000,
        "M": 1000000,
        "B": 1000000000,
    }
    try:
        # Split the numeric part and the suffix
        num = float(param_size[:-1])  # Extract the number (e.g., "1.5")
        suffix = param_size[-1].upper()  # Extract the suffix (e.g., "B")
        return int(num * suffix_multipliers.get(suffix, 1))  # Default multiplier is 1
    except (ValueError, KeyError):
        raise ValueError(f"Invalid parameter size format: {param_size}")


def get_model(args) -> int:
    """Select a model and extract parameter information. Returns max tokens."""
    models = get_models(args)
    if len(models) > 1:
        if args.debug:
            logger.debug(
                "Multiple models found matching '%s':\n%s",
                args.model,
                json.dumps(models, indent=2),
            )
            logger.debug("Using the first model found: %s", models[0]["name"])
    model = models[0]
    if args.debug:
        logger.debug("Using model:\n%s", json.dumps(model, indent=2))
    # Convert parameter_size to max_tokens
    try:
        max_tokens = parse_parameter_size(model["details"]["parameter_size"])
    except Exception as e:
        if args.debug:
            logger.debug("%s", json.dumps(model, indent=2))
        raise ValueError(f"Invalid parameter size format: {model['details']['parameter_size']}")
    args.model = model["name"]
    return max_tokens
