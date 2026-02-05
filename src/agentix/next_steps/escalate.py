from agentix import AgentixConfig
from agentix.context.message import Message
from agentix.prompt_classification_response import NextStep


def escalate(
    args: AgentixConfig, next_step: NextStep, history: list[Message], max_tokens: int
) -> str:
    """
    Docstring for escalate

    :param args: Description
    :type args: AgentixConfig
    :param next_step: Description
    :type next_step: NextStep
    :param history: Description
    :type history: list[Message]
    :param max_tokens: Description
    :type max_tokens: int
    :return: Description
    :rtype: str
    """
    pass
