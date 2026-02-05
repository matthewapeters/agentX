"""agentix.tools.describe_tools.tool_extractor.py"""

from __future__ import annotations

from typing import Dict, List

import libcst as cst

from .tool_collector import _ToolCollector


class ToolExtractor:
    """
    Docstring for ToolExtractor
    """

    def __init__(self, debug: bool = False):
        self.debug = debug
        self.tools = []

    def from_code(self, source: str) -> List[Dict]:
        """
        Parse Python source with LibCST and extract a list of tool specs (dicts) for
        top-level functions and class methods (non-nested).
        """
        module = cst.parse_module(source)
        collector = _ToolCollector(module, debug=self.debug)
        module.visit(collector)
        self.tools = collector.tools
        return self.tools

    def from_file(self, path: str) -> List[Dict]:
        """
        Docstring for extract_tools_from_file

        :param self: Description
        :param path: Description
        :type path: str
        :return: Description
        :rtype: List[Dict]
        """
        with open(path, "r", encoding="utf8") as f:
            source = f.read()
        return self.from_code(source)
