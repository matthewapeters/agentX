import logging

import litellm
from smolagents import CodeAgent, DuckDuckGoSearchTool, LiteLLMModel, RunResult

from agentix.constants import OLLAMA_API_BASE

# Configure logging
logging.basicConfig(level=logging.DEBUG)
logger = logging.getLogger(__name__)

litellm._turn_on_debug()


def webquery(query: str, model: str, ollama_host: str = "localhost:11434") -> RunResult | None:
    """
    Run a web search query using a CodeAgent backed by the given Ollama model.

    :param query: The search query to run.
    :param model: Ollama model name (e.g. "llama3.2:latest"). Read from AgentixConfig.model.
    :param ollama_host: Ollama host in "host:port" format. Defaults to "localhost:11434".
    :return: The agent RunResult, or None if an error occurred.
    """
    llm = LiteLLMModel(
        model_id=f"ollama_chat/{model}",
        api_base=f"http://{ollama_host}",
        temperature=0.7,
        stream=True,
    )

    agent = CodeAgent(tools=[DuckDuckGoSearchTool()], model=llm)

    try:
        result: RunResult = agent.run(query)
        return result
    except Exception as e:
        logger.error("Error occurred while running webquery agent: %s", e, exc_info=True)
        logger.debug("Tool call details: query=%s tools=%s", query, agent.tools)
        return None


def codeagent(query: str, model: str, ollama_host: str = "localhost:11434") -> RunResult | None:
    """
    Run a coding query using a CodeAgent backed by the given Ollama model.

    :param query: The coding query to run.
    :param model: Ollama model name (e.g. "llama3.2:latest"). Read from AgentixConfig.model.
    :param ollama_host: Ollama host in "host:port" format. Defaults to "localhost:11434".
    :return: The agent RunResult, or None if an error occurred.
    """
    llm = LiteLLMModel(
        model_id=f"ollama_chat/{model}",
        api_base=f"http://{ollama_host}",
        temperature=0.7,
        stream=True,
    )

    agent = CodeAgent(tools=[], model=llm)

    try:
        result: RunResult = agent.run(query)
        return result
    except Exception as e:
        logger.error("Error occurred while running codeagent: %s", e, exc_info=True)
        logger.debug("Tool call details: query=%s tools=%s", query, agent.tools)
        return None
