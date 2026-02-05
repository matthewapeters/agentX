"""Tool collector for extracting tool specifications from Python code."""

from __future__ import annotations

import sys
from typing import Dict, List, Optional

import libcst as cst

from .tool_spec import ToolSpec
from .utils import _docstring_summary, _extract_docstring_from_function


class _ToolCollector(cst.CSTVisitor):
    """
    Docstring for _ToolCollector
    """

    def __init__(self, module: cst.Module, debug: bool = False):
        self.module = module
        self.tools: List[ToolSpec] = []
        self._class_stack: List[str] = []
        self._func_depth: int = 0
        self.debug = debug

    def visit_FunctionDef(self, node: cst.FunctionDef) -> Optional[bool]:
        if self.debug:
            print(f"Visiting function: {node.name.value}", file=sys.stderr)
        # Only collect top-level functions and class methods (not nested functions)
        if self._func_depth == 0:
            is_method = len(self._class_stack) > 0
            class_name = self._class_stack[-1] if is_method else None
            qname = (
                ".".join(self._class_stack + [node.name.value])
                if is_method
                else node.name.value
            )

            doc = _extract_docstring_from_function(node)
            desc = _docstring_summary(doc)

            params_schema = self._extract_params_schema(node)
            ret_schema = self._extract_return_schema(node)

            spec = ToolSpec(
                name=node.name.value,
                description=desc,
                docstring=doc,
                parameters_schema=params_schema,
                returns=ret_schema,
                qualified_name=qname,
                is_method=is_method,
                class_name=class_name,
            )
            self.tools.append(spec)
        self._func_depth += 1
        return True

    def leave_FunctionDef(self, node: cst.FunctionDef) -> None:
        self._func_depth -= 1

    def visit_ClassDef(self, node: cst.ClassDef) -> Optional[bool]:
        self._class_stack.append(node.name.value)
        return True

    def leave_ClassDef(self, node: cst.ClassDef) -> None:
        if self._class_stack:
            self._class_stack.pop()

    def _extract_params_schema(self, node: cst.FunctionDef) -> Dict:
        """Extract parameter schema from function definition."""
        params = {}
        for param in node.params.params:
            param_name = param.name.value
            annotation = param.annotation.annotation if param.annotation else None
            param_type = (
                self.module.code_for_node(annotation) if annotation else "string"
            )
            params[param_name] = {"type": param_type}
        return {"properties": params}

    def _extract_return_schema(self, node: cst.FunctionDef) -> Optional[Dict]:
        """Extract return type schema from function definition."""
        if node.returns and node.returns.annotation:
            return_type = self.module.code_for_node(node.returns.annotation)
            return {"type": return_type}
        return None

    def leave_FunctionDef(self, node: cst.FunctionDef) -> None:
        """
        Docstring for leave_FunctionDef

        :param self: Description
        :param node: Description
        :type node: cst.FunctionDef
        """

        # decrement depth
        self._func_depth -= 1
        if self._func_depth == 0:
            self._in_func = False
