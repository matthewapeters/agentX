# API client for Agentix CLI

import json
import logging
import sys
from typing import Iterator

import requests

from .agentix_config import AgentixConfig
from .constants import OLLAMA_API_BASE, OLLAMA_CHAT_ENDPOINT
from .context.prompts import get_user_prompt
from .query_payload import QueryPayload

# from .sessions import update_session

logger = logging.getLogger(__name__)


def _extract_json_payload(text: str) -> str:
    """
    Extract JSON from LLM response, handling markdown code blocks and preamble.

    Common LLM response patterns:
    1. Raw JSON: {"key": "value"}
    2. Markdown: ```json\n{"key": "value"}\n```
    3. With preamble: "Here's the result:\n```\n{...}\n```"
    4. No start fence: Some text {"key": "value"} more text
    5. Pretty-printed with leading newlines: \n{\n  "key": "value"\n}
    6. Markdown without newline: ```json{"key":"value"}```
    7. Combined: \n```json\n{...}\n```

    Returns the extracted JSON string, or raises ValueError if no valid JSON found.
    """
    # Step 1: Strip ALL leading/trailing whitespace (including newlines)
    cleaned = text.strip()

    # Remove special tokens
    if cleaned.startswith("<|assistant|>"):
        cleaned = cleaned[len("<|assistant|>") :].lstrip()

    # Step 2: Handle markdown code blocks more flexibly
    if "```" in cleaned:
        # Try to extract content between first ``` and last ```
        first_fence = cleaned.find("```")
        last_fence = cleaned.rfind("```")

        if first_fence != -1 and last_fence != -1 and first_fence < last_fence:
            # Get content between fences (may have language identifier immediately after opening fence)
            between_fences = cleaned[first_fence + 3 : last_fence]

            # Aggressively strip whitespace
            between_fences = between_fences.strip()

            # Check if first line is a language identifier
            # Handle both:
            #   ```json\n{...}     (identifier on same line as fence)
            #   ```\njson\n{...}   (identifier on next line - rare but possible)
            first_word = between_fences.split()[0] if between_fences.split() else ""
            first_word_lower = first_word.lower()

            # Known code languages to skip
            if first_word_lower in ["bash", "python", "py", "sh", "shell", "javascript", "js", "typescript", "ts"]:
                logger.debug(
                    f"Skipping {first_word_lower} code block, not JSON", extra={"code_block_language": first_word_lower}
                )
                # Fall through to try finding JSON in the raw text
                cleaned = text.strip()
            elif first_word_lower in ["json", "jsonc"]:
                # Remove the language identifier and keep the rest
                cleaned = between_fences[len(first_word) :].lstrip()
            else:
                # No language identifier, or unknown - treat as potential JSON
                cleaned = between_fences
        else:
            # Malformed fences - just remove all backticks
            cleaned = cleaned.replace("```", "").strip()

    # Step 3: Remove standalone "json" or "jsonc" line at start (sometimes appears without fences)
    # This handles: json\n{\n  "key": "value"\n}
    if cleaned and not cleaned.startswith(("{", "[")):
        lines = cleaned.splitlines()
        if lines and lines[0].strip().lower() in ["json", "jsonc"]:
            cleaned = "\n".join(lines[1:]).strip()

    # Step 4: Extract just the JSON object if surrounded by text
    # This handles: "Here is the result: {...} Hope this helps!"
    # Be aggressive about finding JSON even if prefix text exists
    if cleaned and not cleaned.startswith(("{", "[")):
        start = cleaned.find("{")
        if start != -1:
            # Find matching closing brace by counting depth
            depth = 0
            for i in range(start, len(cleaned)):
                if cleaned[i] == "{":
                    depth += 1
                elif cleaned[i] == "}":
                    depth -= 1
                    if depth == 0:
                        cleaned = cleaned[start : i + 1]
                        break
        else:
            # Try array syntax
            start = cleaned.find("[")
            if start != -1:
                depth = 0
                for i in range(start, len(cleaned)):
                    if cleaned[i] == "[":
                        depth += 1
                    elif cleaned[i] == "]":
                        depth -= 1
                        if depth == 0:
                            cleaned = cleaned[start : i + 1]
                            break

    # Step 5: Final aggressive whitespace strip (handles pretty-printed JSON with leading newlines)
    cleaned = cleaned.strip()

    # Step 6: Validate the result looks like JSON
    if not cleaned:
        logger.error(
            "Empty string after extraction",
            extra={"original_text_length": len(text), "original_text_preview": text[:200]},
        )
        raise ValueError("No JSON found in LLM response (empty after extraction)")

    if not (cleaned.startswith("{") or cleaned.startswith("[")):
        logger.error(
            "Extracted text does not look like JSON",
            extra={
                "extracted_text_preview": cleaned[:200],
                "extracted_text_length": len(cleaned),
                "starts_with": cleaned[0] if cleaned else None,
            },
        )
        # Last resort: try to find any JSON in the original text
        json_start = text.find("{")
        if json_start != -1:
            logger.info("Attempting last-resort JSON extraction from original text")
            depth = 0
            for i in range(json_start, len(text)):
                if text[i] == "{":
                    depth += 1
                elif text[i] == "}":
                    depth -= 1
                    if depth == 0:
                        cleaned = text[json_start : i + 1]
                        logger.info("Extracted JSON from original text", extra={"extracted_length": len(cleaned)})
                        break

        # Final check
        if not (cleaned.startswith("{") or cleaned.startswith("[")):
            raise ValueError(
                f"No valid JSON found in LLM response. "
                f"Response appears to be conversational text instead of structured JSON. "
                f"First 100 chars: {text[:100]}"
            )

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
    ollama_base = f"http://{args.ollama_host}" if hasattr(args, "ollama_host") and args.ollama_host else OLLAMA_API_BASE

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

        # Log what we're about to parse
        logger.debug(
            "Extracted JSON payload for parsing",
            extra={
                "raw_answer_length": len(answer),
                "cleaned_length": len(agent_content_clean),
                "cleaned_preview": agent_content_clean[:200] if agent_content_clean else "(empty)",
            },
        )

        # Handle empty or invalid JSON
        if not agent_content_clean or not agent_content_clean.strip():
            logger.error(
                "Empty JSON payload after extraction",
                extra={
                    "raw_answer": answer[:500],
                    "finish_reason": finish_reason,
                },
            )
            return {}

        try:
            return json.loads(agent_content_clean)
        except json.JSONDecodeError as e:
            logger.error(
                "JSON parse error",
                extra={
                    "error": str(e),
                    "error_pos": e.pos,
                    "error_lineno": e.lineno,
                    "error_colno": e.colno,
                    "cleaned_payload_length": len(agent_content_clean),
                    "cleaned_payload_repr": repr(agent_content_clean),
                    "cleaned_payload_full": agent_content_clean,
                    "raw_answer_length": len(answer),
                    "raw_answer_repr": repr(answer),
                    "raw_answer_first_500": answer[:500],
                    "raw_answer_last_500": answer[-500:] if len(answer) > 500 else answer,
                },
            )
            raise
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


def query_api_streaming(args: AgentixConfig, payload: QueryPayload) -> Iterator[dict]:
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
    ollama_base = f"http://{args.ollama_host}" if hasattr(args, "ollama_host") and args.ollama_host else OLLAMA_API_BASE

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
                            print(f"Error position: line {e.lineno}, col {e.colno}", file=sys.stderr)
                            print(f"Line length: {len(line_str)}", file=sys.stderr)
                            print(f"Line repr: {repr(line_str)}", file=sys.stderr)
                            print(f"Line content (first 200): {line_str[:200]}", file=sys.stderr)
                            if len(line_str) > 200:
                                print(f"Line content (last 200): {line_str[-200:]}", file=sys.stderr)
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
