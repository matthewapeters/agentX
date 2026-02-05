"""Main functionality for Agentix CLI application."""

import json
import sys
from typing import Optional

from .agentix_config import AgentixConfig
from .api_client import query_api
from .context.sessions import assemble_classification_prompt, manage_sessions
from .models import get_model
from .next_steps import take_steps
from .prompt_classification_response import PromptClassificationResponse


def agentix(args: AgentixConfig) -> Optional[dict]:
    """Main entry point for Agentix CLI application."""
    # Get model and set max_tokens
    max_tokens = get_model(args)

    # Manage session state
    print(f"Debug: args.session = {args.session}", file=sys.stderr)
    print("Debug: Invoking manage_sessions", file=sys.stderr)
    history = manage_sessions(args)

    # Assemble payload and query API
    initial_prompt = assemble_classification_prompt(args, history, max_tokens)

    # Query API and get classification
    classification: dict = query_api(args, initial_prompt)
    if args.debug:
        print(
            json.dumps(classification, indent=2)
            .encode("utf-8")
            .decode("unicode_escape"),
            file=sys.stderr,
        )
    prompt_classiication: PromptClassificationResponse = None
    try:
        prompt_classiication = PromptClassificationResponse(**classification)
    except Exception as e:
        print(f"Error parsing classification response: {e}", file=sys.stderr)
        if args.debug:
            print(classification, file=sys.stderr)

    # take the steps indicated from the LLM
    take_steps(args, prompt_classiication.next_step, history, max_tokens)
