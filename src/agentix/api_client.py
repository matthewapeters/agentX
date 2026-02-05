# API client for Agentix CLI

import json
import sys

import requests

from .agentix_config import AgentixConfig
from .constants import OLLAMA_API_BASE, OLLAMA_CHAT_ENDPOINT
from .context.prompts import get_user_prompt
from .query_payload import QueryPayload

# from .sessions import update_session


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
        print(json.dumps(payload, indent=2), file=sys.stderr)

    response = requests.post(
        f"{OLLAMA_API_BASE}{OLLAMA_CHAT_ENDPOINT}",
        headers=headers,
        data=json.dumps(payload),
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
        agent_content_clean = answer.replace("\n", "").replace("\t", "")
        return json.loads(agent_content_clean)
    else:
        print("Error:", response.status_code, response.text)
        return {}


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
