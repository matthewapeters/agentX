"""
Advanced code analysis tools using CST and AST.

Provides code structure analysis, refactoring suggestions,
and code understanding capabilities for Python files.
"""

import ast
import inspect
from typing import Any, Dict, List, Optional


class CodeAnalyzer:
    """Analyze Python code structure using AST."""

    def __init__(self, code: str):
        """
        Initialize analyzer with code.

        Args:
            code: Python source code to analyze
        """
        self.code = code
        try:
            self.tree = ast.parse(code)
        except SyntaxError as e:
            self.tree = None
            self.syntax_error = e

    def analyze_syntax(self) -> Dict[str, Any]:
        """
        Analyze code syntax and structure.

        Returns:
            Dictionary with syntax information
        """
        if self.tree is None:
            return {
                "valid": False,
                "error": str(self.syntax_error),
                "line": getattr(self.syntax_error, "lineno", None),
                "offset": getattr(self.syntax_error, "offset", None),
            }

        return {
            "valid": True,
            "line_count": len(self.code.splitlines()),
            "ast_nodes": self._count_nodes(),
            "imports": len(self._find_imports()),
            "functions": len(self._find_functions()),
            "classes": len(self._find_classes()),
        }

    def find_functions(self) -> List[Dict[str, Any]]:
        """
        Find all function definitions in code.

        Returns:
            List of function information
        """
        if self.tree is None:
            return []

        functions = []

        for node in ast.walk(self.tree):
            if isinstance(node, ast.FunctionDef):
                func_info = {
                    "name": node.name,
                    "line": node.lineno,
                    "decorators": [self._node_to_str(d) for d in node.decorator_list],
                    "arguments": self._get_function_args(node),
                    "returns": "Yes" if node.returns else "No",
                    "docstring": ast.get_docstring(node),
                }
                functions.append(func_info)

        return functions

    def find_classes(self) -> List[Dict[str, Any]]:
        """
        Find all class definitions and their methods.

        Returns:
            List of class information
        """
        if self.tree is None:
            return []

        classes = []

        for node in ast.walk(self.tree):
            if isinstance(node, ast.ClassDef):
                methods = []
                for item in node.body:
                    if isinstance(item, ast.FunctionDef):
                        methods.append(
                            {
                                "name": item.name,
                                "line": item.lineno,
                                "is_private": item.name.startswith("_"),
                            }
                        )

                class_info = {
                    "name": node.name,
                    "line": node.lineno,
                    "bases": [self._node_to_str(b) for b in node.bases],
                    "methods": methods,
                    "method_count": len(methods),
                    "docstring": ast.get_docstring(node),
                }
                classes.append(class_info)

        return classes

    def find_imports(self) -> List[Dict[str, str]]:
        """
        Find all imports in the code.

        Returns:
            List of import information
        """
        if self.tree is None:
            return []

        imports = []

        for node in ast.walk(self.tree):
            if isinstance(node, ast.Import):
                for alias in node.names:
                    imports.append(
                        {
                            "type": "import",
                            "module": alias.name,
                            "alias": alias.asname,
                        }
                    )
            elif isinstance(node, ast.ImportFrom):
                for alias in node.names:
                    imports.append(
                        {
                            "type": "from",
                            "module": node.module or ".",
                            "name": alias.name,
                            "alias": alias.asname,
                        }
                    )

        return imports

    def suggest_refactoring(self) -> List[Dict[str, str]]:
        """
        Suggest code refactoring opportunities.

        Returns:
            List of refactoring suggestions
        """
        if self.tree is None:
            return []

        suggestions = []

        # Check for long functions
        for node in ast.walk(self.tree):
            if isinstance(node, ast.FunctionDef):
                func_lines = self._count_node_lines(node)
                if func_lines > 50:
                    suggestions.append(
                        {
                            "type": "function_size",
                            "severity": "warning",
                            "location": f"Function '{node.name}' at line {node.lineno}",
                            "suggestion": f"Function is {func_lines} lines. Consider breaking into smaller functions.",
                        }
                    )

                # Check for too many arguments
                args = len(node.args.args)
                if args > 5:
                    suggestions.append(
                        {
                            "type": "too_many_args",
                            "severity": "info",
                            "location": f"Function '{node.name}' at line {node.lineno}",
                            "suggestion": f"Function has {args} arguments. Consider using a dataclass or dict.",
                        }
                    )

        # Check for unused imports
        imports = {imp["module"] for imp in self.find_imports() if imp["type"] == "import"}
        for imp in imports:
            if not self._is_import_used(imp):
                suggestions.append(
                    {
                        "type": "unused_import",
                        "severity": "warning",
                        "suggestion": f"Import '{imp}' appears unused.",
                    }
                )

        return suggestions

    def _find_imports(self) -> list:
        """Find all import nodes."""
        imports = []
        for node in ast.walk(self.tree):
            if isinstance(node, (ast.Import, ast.ImportFrom)):
                imports.append(node)
        return imports

    def _find_functions(self) -> list:
        """Find all function definition nodes."""
        functions = []
        for node in ast.walk(self.tree):
            if isinstance(node, ast.FunctionDef):
                functions.append(node)
        return functions

    def _find_classes(self) -> list:
        """Find all class definition nodes."""
        classes = []
        for node in ast.walk(self.tree):
            if isinstance(node, ast.ClassDef):
                classes.append(node)
        return classes

    def _count_nodes(self) -> int:
        """Count total AST nodes."""
        return len(list(ast.walk(self.tree))) if self.tree else 0

    def _count_node_lines(self, node) -> int:
        """Count lines in an AST node."""
        if hasattr(node, "end_lineno") and hasattr(node, "lineno"):
            return node.end_lineno - node.lineno + 1
        return 0

    def _get_function_args(self, node) -> List[str]:
        """Get function arguments."""
        return [arg.arg for arg in node.args.args]

    def _node_to_str(self, node) -> str:
        """Convert AST node to string."""
        try:
            return ast.unparse(node)
        except Exception:
            return str(node)

    def _is_import_used(self, import_name: str) -> bool:
        """Check if an import is used in code."""
        # Simple heuristic: check if the import name appears in the code
        # excluding the import statement itself
        for line in self.code.splitlines():
            if "import" not in line and import_name in line:
                return True
        return False


# Tool factory functions


def execute_analyze_syntax(code: str) -> Dict[str, Any]:
    """Execute analyze_syntax tool."""
    analyzer = CodeAnalyzer(code)
    result = analyzer.analyze_syntax()
    return {"success": True, "data": result}


def execute_find_functions(code: str) -> Dict[str, Any]:
    """Execute find_functions tool."""
    analyzer = CodeAnalyzer(code)
    functions = analyzer.find_functions()
    return {"success": True, "count": len(functions), "data": functions}


def execute_find_classes(code: str) -> Dict[str, Any]:
    """Execute find_classes tool."""
    analyzer = CodeAnalyzer(code)
    classes = analyzer.find_classes()
    return {"success": True, "count": len(classes), "data": classes}


def execute_find_imports(code: str) -> Dict[str, Any]:
    """Execute find_imports tool."""
    analyzer = CodeAnalyzer(code)
    imports = analyzer.find_imports()
    return {"success": True, "count": len(imports), "data": imports}


def execute_suggest_refactoring(code: str) -> Dict[str, Any]:
    """Execute suggest_refactoring tool."""
    analyzer = CodeAnalyzer(code)
    suggestions = analyzer.suggest_refactoring()
    return {"success": True, "count": len(suggestions), "data": suggestions}
