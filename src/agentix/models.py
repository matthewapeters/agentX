# Model management for Agentix CLI

import json
import sys

import requests

from .constants import OLLAMA_API_BASE, OLLAMA_MODELS_ENDPOINT


def get_models(args):
    """Fetch available models from Ollama API."""
    result = requests.get(f"{OLLAMA_API_BASE}{OLLAMA_MODELS_ENDPOINT}")
    models_json = result.json()

    if args.debug:
        print("Available models:", file=sys.stderr)
        print(json.dumps(models_json, indent=2), file=sys.stderr)
        print(f"Filtering models with prefix: {args.model}", file=sys.stderr)

    models = [
        m
        for m in models_json["models"]
        # filter based on model_name if provided
        if (args.model and m["name"].startswith(args.model)) or (not args.model)
    ]
    # return the first matching model or default to the first model
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
            print(
                f"Multiple models found matching '{args.model}':\n{json.dumps(models, indent=2)}",
                file=sys.stderr,
            )
            print(f"Using the first model found: {models[0]['name']}", file=sys.stderr)
    model = models[0]
    if args.debug:
        print(f"Using model:\n{json.dumps(model, indent=2)}", file=sys.stderr)
    # Convert parameter_size to max_tokens
    try:
        max_tokens = parse_parameter_size(model["details"]["parameter_size"])
    except Exception as e:
        if args.debug:
            print(json.dumps(model, indent=2), file=sys.stderr)
        raise ValueError(
            f"Invalid parameter size format: {model['details']['parameter_size']}"
        )
    args.model = model["name"]
    return max_tokens
