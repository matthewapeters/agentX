from dataclasses import dataclass
from enum import Enum
from typing import Any, Optional

PlanAction = Enum("PlanActions", ("tool", "internal"))

AssertionType = Enum(
    "AssertionTypes", ("exists", "not_empty", "gte", "lte", "equals", "regex")
)


@dataclass
class Assertion:
    def __init__(self, assertion_type: str, on: Any, condition: str):
        self.assertion_type = assertion_type
        self.subject = on
        self.condition = condition

    def assert_condition(self):
        """
        Docstring for assert_condition

        :param self: Description
        """
        match self.assertion_type:
            case AssertionType.exists:
                return self.subject is not None and len(self.subject) > 0
            case AssertionType.not_empty:
                return self.subject is not None and len(self.subject) > 0
            case AssertionType.gte:
                return self.subject is not None and self.subject >= self.condition
            case AssertionType.lte:
                return self.subject is not None and self.subject <= self.condition
            case AssertionType.equals:
                return self.subject is not None and self.subject == self.condition
            case AssertionType.regex:
                import re

                return bool(re.match(self.condition, self.subject))


@dataclass
class PlanStep:
    def __init__(
        self,
        id: str,
        action: PlanAction,
        tool: Optional[str],
        inputs: dict,
        expected_outputs: dict,
        assertions: list[Assertion],
    ):
        self.id: int = int(id.split("-")[1])
        self.action = action
        self.tool = tool
        self.inputs = inputs
        self.expected_outputs = expected_outputs
        self.assertions = assertions
        self._completed = False

    def check_completion(self):
        for assertion in self.assertions:
            if not assertion.assert_condition():
                self.completed = False
                return
        self.completed = True

    @property
    def completed(self) -> bool:
        if not self._completed:
            self.check_completion()
        return self._completed

    def do_action(self):
        match self.action:
            case PlanAction.tool:
                if (
                    self.tool is None
                    or self.inputs is None
                    or self.expected_outputs is None
                ):
                    raise ValueError(
                        "Missing tool, inputs, or expected outputs for a tool action."
                    )
                self.tool(self.inputs, self.expected_outputs)
                return
            case PlanAction.internal:
                # Call the LLM to perform this task
                return
            case _:
                raise ValueError(f"Invalid action: {self.action}")
