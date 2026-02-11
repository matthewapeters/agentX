# API client for Agentix CLI

import json
import sys
from typing import Iterator

import requests

from .agentix_config import AgentixConfig
from .constants import OLLAMA_API_BASE, OLLAMA_CHAT_ENDPOINT
from .context.prompts import get_user_prompt
from .query_payload import QueryPayload

# from .sessions import update_session


def _extract_json_payload(text: str) -> str:
    cleaned = text.strip()

    if cleaned.startswith("<|assistant|>"):
        cleaned = cleaned[len("<|assistant|>"):].lstrip()

    if "```" in cleaned:
        parts = cleaned.split("```")
        if len(parts) >= 3:
            cleaned = parts[1].strip()
        else:
            cleaned = cleaned.replace("```", "").strip()

    lines = cleaned.splitlines()
    if lines and lines[0].strip() == "json":
        cleaned = "\n".join(lines[1:]).strip()

    return cleaned


def _get_latest_user_prompt(payload: QueryPayload | dict) -> str:
    payload_dict = payload if isinstance(payload, dict) else payload.to_dict()
    messages = payload_dict.get("messages", [])
    for message in reversed(messages):
        if message.get("role") == "user":
            return message.get("content") or ""
    return ""


def query_api(args: AgentixConfig, payload: QueryPayload) -> dict:
    """
    Send request to Ollama API and parse response.

    params:
        args (AgentixConfig): Configuration for the agent
        payload (dict): Payload to send to Ollama API - this is a structured dict of the
            context and other information

    """
    headers = {
        "Content-Type": "application/json",
    }

    if args.debug:
        print("Payload:", file=sys.stderr)
        print(json.dumps(payload.to_dict(), indent=2), file=sys.stderr)

    # Use configured host or fallback to constant
    ollama_base = f"http://{args.ollama_host}" if hasattr(args, 'ollama_host') and args.ollama_host else OLLAMA_API_BASE
    
    response = requests.post(
        f"{ollama_base}{OLLAMA_CHAT_ENDPOINT}",
        headers=headers,
        data=json.dumps(payload.to_dict()),
        timeout=300,
    )

    if response.status_code == 200:
        result = response.json()

        if args.debug:
            print("Raw response:", file=sys.stderr)
            print(json.dumps(result, indent=2), file=sys.stderr)

        answer = result["choices"][0]["message"]["content"]
        reasoning = result["choices"][0]["message"].get("reasoning", "")
        finish_reason = result["choices"][0].get("finish_reason", "")

        if args.debug:
            print("Finish reason:", finish_reason, file=sys.stderr)
            print("Response:", file=sys.stderr)
            print(answer, file=sys.stderr)
            print("\nReasoning:", file=sys.stderr)
            print(reasoning, file=sys.stderr)

        # update_session(args, payload["messages"], answer)
        agent_content_clean = _extract_json_payload(answer)
        return json.loads(agent_content_clean)
    else:
        print("Error:", response.status_code, response.text)
        return {}


def query_classification(args: AgentixConfig, payload: QueryPayload | dict) -> dict:
    """Classify intent using the configured backend.

    This keeps query_api intact for the baseline Ollama path.
    """
    backend = getattr(args, "classification_backend", "ollama")
    if backend == "torch":
        from .local_classifier import classify_intent_with_torch

        prompt_text = _get_latest_user_prompt(payload)
        if not prompt_text:
            return {
                "intent": "conversation",
                "needs_clarification": False,
                "missing_fields": [],
                "reasoning_summary": "torch-zero-shot:empty prompt",
                "next_step": "respond_directly",
            }

        return classify_intent_with_torch(
            prompt_text,
            getattr(args, "classification_torch_model", None),
            getattr(args, "classification_torch_device", None),
        )

    return query_api(args, payload)


def summarize_user_prompt(args: AgentixConfig) -> str:
    """Generate a session summary name based on the user prompt."""
    # Use query_api to generate a session summary name based on the user prompt
    summary_payload = {
        "model": "phi4-mini:3.8b",  # args.model,
        "messages": [
            {
                "role": "system",
                "content": (
                    "You are an assistant that generates concise session names based on prompts.\n"
                    "Generate a short, descriptive session name (3-5 words) that captures the "
                    "essence of the user's prompt.\n"
                    "Avoid using special characters or spaces in the session name.\n"
                    "Respond with only the session name without any additional text."
                ),
            },
            {"role": "user", "content": get_user_prompt(args)},
        ],
        "temperature": 0.8,
    }
    response = query_api(args, summary_payload)
    # Clean up the response to create a valid session ID
    session_id = response.strip().replace(" ", "_").replace("/", "_")
    args.session = session_id


def query_api_streaming(
    args: AgentixConfig, 
    payload: QueryPayload
) -> Iterator[dict]:
    """
    Stream responses from Ollama API.
    
    This function sends a request to Ollama with streaming enabled
    and yields response chunks as they arrive.
    
    Args:
        args: AgentixConfig with model and settings
        payload: QueryPayload to send to API
        
    Yields:
        Response chunk dictionaries from Ollama
        
    Example:
        for chunk in query_api_streaming(config, payload):
            if chunk.get("done"):
                break
            content = chunk.get("message", {}).get("content", "")
            print(content, end="", flush=True)
    """
    headers = {"Content-Type": "application/json"}
    
    # Convert payload to dict and enable streaming
    payload_dict = payload if isinstance(payload, dict) else payload.to_dict()
    payload_dict["stream"] = True
    
    if args.debug:
        print("Streaming payload:", file=sys.stderr)
        print(json.dumps(payload_dict, indent=2), file=sys.stderr)
    
    # Use configured host or fallback to constant
    ollama_base = f"http://{args.ollama_host}" if hasattr(args, 'ollama_host') and args.ollama_host else OLLAMA_API_BASE
    
    try:
        response = requests.post(
            f"{ollama_base}{OLLAMA_CHAT_ENDPOINT}",
            headers=headers,
            json=payload_dict,
            timeout=300,
            stream=True,  # Enable streaming mode
        )
        
        if response.status_code == 200:
            # Iterate over lines in the response
            for line in response.iter_lines():
                if line:
                    try:
                        # Decode line
                        line_str = line.decode("utf-8")
                        
                        # Strip SSE "data: " prefix if present
                        if line_str.startswith("data: "):
                            line_str = line_str[6:]  # Remove "data: " prefix
                        
                        # Skip empty lines after stripping prefix
                        if not line_str.strip():
                            continue
                        
                        chunk = json.loads(line_str)
                        
                        if args.debug:
                            print(f"Chunk: {chunk}", file=sys.stderr)
                        
                        yield chunk
                        
                        # Stop if done
                        if chunk.get("done", False):
                            break
                    except json.JSONDecodeError as e:
                        if args.debug:
                            print(f"JSON decode error: {e}", file=sys.stderr)
                            print(f"Line was: {line_str[:100]}", file=sys.stderr)
                        continue
        else:
            # Yield error chunk
            yield {
                "error": f"HTTP {response.status_code}: {response.text}",
                "done": True,
            }
    except Exception as e:
        # Yield exception as error chunk
        yield {
            "error": f"Request failed: {str(e)}",
            "done": True,
        }
