"""agentix/transforms.py"""

import json
import sys


def transform_ollama_tags_to_openai_engines(ollama_tags, filter_tag=None):
    """
    Transforms the Ollama API /tags output schema into the OpenAI /v1/engines schema.

    Args:
        ollama_tags (list): A list of dictionaries representing the Ollama /tags output schema.
            Example:
            [
                {"id": "tag1", "name": "Model 1", "description": "Description of Model 1"},
                {"id": "tag2", "name": "Model 2", "description": "Description of Model 2"}
            ]
        filter_tag (str, optional): If provided, only include the tag with this ID.

    Returns:
        dict: A dictionary representing the OpenAI /v1/engines schema.
            Example:
            {
                "data": [
                    {"id": "tag1", "object": "engine", "owner": "ollama", "ready": True},
                    {"id": "tag2", "object": "engine", "owner": "ollama", "ready": True}
                ]
            }
    """
    print(json.dumps(ollama_tags, indent=2), file=sys.stderr)
    return {
        "data": [
            {"id": tag["name"], "object": "engine", "owner": "ollama", "ready": True}
            for tag in ollama_tags
            if filter_tag is None or tag["name"] == filter_tag
        ]
    }
