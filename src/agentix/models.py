# Model management for Agentix CLI

import json
import logging

import requests

from .constants import FALLBACK_CONTEXT_WINDOW, OLLAMA_API_BASE, OLLAMA_MODELS_ENDPOINT

logger = logging.getLogger(__name__)


def get_models(args, filter_by_model: bool = True) -> list[dict]:
    """
    Fetch available models from Ollama API.

    Args:
        args: AgentixConfig with model and ollama_host settings
        filter_by_model: If True, filter to models matching args.model; if False, return all models
    """
    # Use configured host or fallback to constant
    ollama_base = f"http://{args.ollama_host}" if hasattr(args, "ollama_host") and args.ollama_host else OLLAMA_API_BASE

    try:
        result = requests.get(f"{ollama_base}{OLLAMA_MODELS_ENDPOINT}", timeout=10)
        result.raise_for_status()
        models_json = result.json()
    except (requests.RequestException, ValueError) as exc:
        logger.error("Failed to fetch models from Ollama: %s", exc)
        return []

    if not isinstance(models_json, dict):
        logger.error("Unexpected models payload type from Ollama: %s", type(models_json).__name__)
        return []

    all_models = models_json.get("models", [])
    if not isinstance(all_models, list):
        logger.error("Unexpected 'models' field type from Ollama: %s", type(all_models).__name__)
        return []

    if args.debug:
        logger.debug("Available models: %s", json.dumps(models_json, indent=2))
        if filter_by_model:
            logger.debug("Filtering models with prefix: %s", args.model)

    # Only filter if requested and model is configured
    if filter_by_model and args.model:
        models = [
            m
            for m in all_models
            # filter based on model_name if provided
            if isinstance(m, dict) and isinstance(m.get("name"), str) and m["name"].startswith(args.model)
        ]
        return models

    return [model for model in all_models if isinstance(model, dict)]


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
    if not param_size or len(param_size) < 2:
        raise ValueError(f"Invalid parameter size format: {param_size}")

    try:
        num = float(param_size[:-1])
    except ValueError as exc:
        raise ValueError(f"Invalid parameter size format: {param_size}") from exc

    suffix = param_size[-1].upper()
    if suffix not in suffix_multipliers:
        raise ValueError(f"Invalid parameter size format: {param_size}")

    return int(num * suffix_multipliers[suffix])


def get_model(args, max_tokens: int | None = None) -> int:
    """Select a model and extract parameter information.  Returns max tokens.

    When *max_tokens* is supplied (non-``None``) the function skips the live
    HTTP call to Ollama and returns that value directly.  This lets callers
    that already hold the context length (e.g. via
    :class:`~agentx.model_metadata_store.ModelMetadataStore`) avoid a
    redundant network round-trip.

    Args:
        args: Parsed argument namespace.  Must have at least ``model`` and
            ``debug`` attributes.
        max_tokens: Optional context-window override.  When ``None`` (default)
            the context length is fetched from the Ollama ``/api/show``
            endpoint.

    Returns:
        Maximum token count for the selected model.
    """
    models = get_models(args)
    if not models:
        requested = getattr(args, "model", None)
        raise RuntimeError(f"No models available from Ollama for selector '{requested}'")

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
    args.model = model["name"]

    # Fast path: caller already knows the context length.
    if max_tokens is not None:
        return max_tokens

    from shared.providers.ollama_provider import OllamaServiceProvider

    ollama_host = args.ollama_host if hasattr(args, "ollama_host") else "localhost:11434"
    provider = OllamaServiceProvider(host=ollama_host)
    resolved = provider.get_context_length(model["name"])
    if resolved == FALLBACK_CONTEXT_WINDOW:
        logger.debug("Using FALLBACK_CONTEXT_WINDOW for model '%s'", model["name"])
    elif args.debug:
        logger.debug("Resolved context length for model '%s': %d", model["name"], resolved)
    return resolved
