import logging

import litellm
from smolagents import CodeAgent, DuckDuckGoSearchTool, LiteLLMModel, RunResult

# Configure logging
logging.basicConfig(level=logging.DEBUG)
logger = logging.getLogger(__name__)

litellm._turn_on_debug()


def webquery(query: str):
    """
    Docstring for webquery

    :param query: Description
    :type query: str
    """
    # TODO: need to get the model from the agentx config instead of hardcoding it here
    model = LiteLLMModel(
        model_id="ollama_chat/llama3.2:latest",
        api_base="http://localhost:11434",
        temperature=0.7,
        stream=True,
    )

    agent = CodeAgent(tools=[DuckDuckGoSearchTool()], model=model)

    # Add logging to capture tool call details
    result: RunResult
    try:
        result = agent.run(query)
        print(result)
        return result
    except Exception as e:
        logger.error(
            f"Error occurred while running the agent: {e}",
            exc_info=True)
        logger.debug(
            "Tool call details:",
            extra={"query": query,
                   "tools": agent.tools})


def codeagent(query: str):
    """
    Docstring for codeagent

    :param query: Description
    :type query: str
    """
    # TODO: need to get the model from the agentx config instead of hardcoding it here
    model = LiteLLMModel(
        model_id="ollama_chat/llama3.2:latest",
        api_base="http://localhost:11434",
        temperature=0.7,
        stream=True,
    )

    agent = CodeAgent(tools=[], model=model)

    # Add logging to capture tool call details
    result: RunResult
    try:
        result = agent.run(query)
        print(result)
        return result
    except Exception as e:
        logger.error(
            f"Error occurred while running the agent: {e}",
            exc_info=True)
        logger.debug(
            "Tool call details:",
            extra={"query": query,
                   "tools": agent.tools})
