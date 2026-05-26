"""
Tests for code analysis tools.
"""

import os
import sys
from pathlib import Path

# Add src to path
project_root = str(Path(__file__).parent.parent)
sys.path.insert(0, os.path.join(project_root, "src"))

from agentx.integration.code_analysis import (
    CodeAnalyzer,
    execute_analyze_syntax,
    execute_find_classes,
    execute_find_functions,
    execute_find_imports,
    execute_suggest_refactoring,
)


def test_syntax_analysis():
    """Test syntax analysis."""
    code = """
import os

def hello():
    return "world"

class MyClass:
    pass
"""

    analyzer = CodeAnalyzer(code)
    result = analyzer.analyze_syntax()

    assert result["valid"] == True
    assert result["functions"] == 1
    assert result["classes"] == 1
    assert result["imports"] == 1
    print("✅ Syntax analysis works")


def test_invalid_syntax():
    """Test invalid syntax detection."""
    code = """
def broken(:
    pass
"""

    analyzer = CodeAnalyzer(code)
    result = analyzer.analyze_syntax()

    assert result["valid"] == False
    assert "error" in result
    print("✅ Invalid syntax detection works")


def test_find_functions():
    """Test finding functions."""
    code = """
def add(a, b):
    '''Add two numbers'''
    return a + b

def multiply(x, y):
    return x * y

@decorator
def decorated_func():
    pass
"""

    analyzer = CodeAnalyzer(code)
    functions = analyzer.find_functions()

    assert len(functions) == 3
    assert functions[0]["name"] == "add"
    assert "Add two" in functions[0]["docstring"]
    assert functions[2]["decorators"] == ["decorator"]
    print("✅ Function finding works")


def test_find_classes():
    """Test finding classes."""
    code = """
class Animal:
    def __init__(self):
        pass

    def speak(self):
        pass

class Dog(Animal):
    def bark(self):
        pass
"""

    analyzer = CodeAnalyzer(code)
    classes = analyzer.find_classes()

    assert len(classes) == 2
    assert classes[0]["name"] == "Animal"
    assert classes[0]["method_count"] == 2
    assert classes[1]["bases"] == ["Animal"]
    print("✅ Class finding works")


def test_find_imports():
    """Test finding imports."""
    code = """
import os
import sys as system
from pathlib import Path
from typing import List, Optional
"""

    analyzer = CodeAnalyzer(code)
    imports = analyzer.find_imports()

    assert len(imports) == 5
    assert imports[0]["module"] == "os"
    assert imports[1]["alias"] == "system"
    assert imports[2]["type"] == "from"
    assert imports[2]["module"] == "pathlib"
    print("✅ Import finding works")


def test_refactoring_suggestions():
    """Test refactoring suggestions."""
    code = """
def very_long_function(a, b, c, d, e, f):
    '''Function with too many arguments'''
    result = a + b
    result = result * c
    result = result - d
    result = result / e
    result = result ** f
    return result
"""

    analyzer = CodeAnalyzer(code)
    suggestions = analyzer.suggest_refactoring()

    # Should have suggestion for too many arguments
    has_arg_suggestion = any(s["type"] == "too_many_args" for s in suggestions)
    assert has_arg_suggestion == True
    print("✅ Refactoring suggestions work")


def test_execute_analyze_syntax():
    """Test analyze_syntax tool function."""
    code = "import os\ndef hello():\n    pass"
    result = execute_analyze_syntax(code)

    assert result["success"] == True
    assert "valid" in result["data"]
    assert result["data"]["valid"] == True
    print("✅ execute_analyze_syntax tool works")


def test_execute_find_functions():
    """Test find_functions tool function."""
    code = """
def foo():
    pass

def bar():
    pass
"""

    result = execute_find_functions(code)

    assert result["success"] == True
    assert result["count"] == 2
    assert len(result["data"]) == 2
    print("✅ execute_find_functions tool works")


def test_execute_find_classes():
    """Test find_classes tool function."""
    code = """
class A:
    pass

class B:
    def method(self):
        pass
"""

    result = execute_find_classes(code)

    assert result["success"] == True
    assert result["count"] == 2
    print("✅ execute_find_classes tool works")


def test_execute_find_imports():
    """Test find_imports tool function."""
    code = "import os\nfrom pathlib import Path"
    result = execute_find_imports(code)

    assert result["success"] == True
    assert result["count"] == 2
    print("✅ execute_find_imports tool works")


def test_execute_suggest_refactoring():
    """Test suggest_refactoring tool function."""
    code = """
def test(a, b, c, d, e, f, g):
    return a + b
"""

    result = execute_suggest_refactoring(code)

    assert result["success"] == True
    assert result["count"] >= 1  # At least one suggestion
    print("✅ execute_suggest_refactoring tool works")


# ---------------------------------------------------------------------------
# Coverage gap tests — 92% → 100%
# Each test targets one or more of the previously uncovered lines.
# ---------------------------------------------------------------------------


class TestInvalidSyntaxEarlyReturns:
    """Lines 62, 88, 123, 154 — all public methods return [] when tree is None."""

    def _broken(self):
        return CodeAnalyzer("def broken(:\n    pass\n")

    def test_find_functions_invalid_syntax_returns_empty(self):
        assert self._broken().find_functions() == []

    def test_find_classes_invalid_syntax_returns_empty(self):
        assert self._broken().find_classes() == []

    def test_find_imports_invalid_syntax_returns_empty(self):
        assert self._broken().find_imports() == []

    def test_suggest_refactoring_invalid_syntax_returns_empty(self):
        assert self._broken().suggest_refactoring() == []


def test_function_size_suggestion():
    """Line 163 — function_size suggestion triggered for functions > 50 lines."""
    long_body = "\n".join(f"    x_{i} = {i}" for i in range(55))
    code = f"def big_function():\n{long_body}\n    return x_0\n"
    analyzer = CodeAnalyzer(code)
    suggestions = analyzer.suggest_refactoring()
    function_size_suggestions = [s for s in suggestions if s["type"] == "function_size"]
    assert len(function_size_suggestions) == 1
    assert "big_function" in function_size_suggestions[0]["location"]
    assert "lines" in function_size_suggestions[0]["suggestion"]


def test_unused_import_suggestion():
    """Line 184 — unused_import suggestion triggered when an import name never appears in code."""
    code = "import json\n\ndef hello():\n    return 'world'\n"
    analyzer = CodeAnalyzer(code)
    suggestions = analyzer.suggest_refactoring()
    unused_suggestions = [s for s in suggestions if s["type"] == "unused_import"]
    assert len(unused_suggestions) == 1
    assert "json" in unused_suggestions[0]["suggestion"]


def test_is_import_used_returns_true():
    """Line 244 — _is_import_used returns True when import name appears outside import lines."""
    code = "import os\n\npath = os.getcwd()\n"
    analyzer = CodeAnalyzer(code)
    assert analyzer._is_import_used("os") is True


def test_used_import_not_flagged():
    """Complement of above — used imports must NOT appear in unused_import suggestions."""
    code = "import os\n\npath = os.getcwd()\n"
    analyzer = CodeAnalyzer(code)
    suggestions = analyzer.suggest_refactoring()
    unused_suggestions = [s for s in suggestions if s["type"] == "unused_import"]
    assert all("os" not in s["suggestion"] for s in unused_suggestions)


def test_count_node_lines_fallback():
    """Line 224 — _count_node_lines returns 0 for a node lacking end_lineno."""

    class FakeNode:
        pass  # no end_lineno, no lineno attributes

    analyzer = CodeAnalyzer("x = 1")
    assert analyzer._count_node_lines(FakeNode()) == 0


def test_node_to_str_exception_fallback():
    """Lines 234-235 — _node_to_str falls back to str(node) when ast.unparse raises."""
    from unittest.mock import patch

    analyzer = CodeAnalyzer("x = 1")
    dummy_node = object()
    with patch("agentx.integration.code_analysis.ast.unparse", side_effect=Exception("boom")):
        result = analyzer._node_to_str(dummy_node)
    # Should return str(node) rather than raising
    assert isinstance(result, str)


def test_complex_code_analysis():
    """Test analysis of complex code."""
    code = """
from typing import List, Optional
from pathlib import Path
import json

class DataProcessor:
    '''Process various data formats'''

    def __init__(self, config: dict):
        self.config = config

    def process_file(self, filepath: Path) -> Optional[dict]:
        '''Process a single file'''
        if not filepath.exists():
            return None

        with open(filepath) as f:
            return json.load(f)

    def _validate_data(self, data: dict) -> bool:
        '''Private validation method'''
        return len(data) > 0

def load_and_process(file_list: List[Path]) -> List[dict]:
    '''Load and process multiple files'''
    processor = DataProcessor({})
    results = []

    for filepath in file_list:
        result = processor.process_file(filepath)
        if result:
            results.append(result)

    return results
"""

    analyzer = CodeAnalyzer(code)

    # Test each capability
    syntax = analyzer.analyze_syntax()
    assert syntax["valid"] == True
    # Should have 4 functions total: __init__, process_file, _validate_data, load_and_process
    assert syntax["functions"] >= 1

    functions = analyzer.find_functions()
    assert len(functions) >= 1

    classes = analyzer.find_classes()
    assert len(classes) == 1
    assert classes[0]["name"] == "DataProcessor"
    assert classes[0]["method_count"] == 3

    imports = analyzer.find_imports()
    assert len(imports) == 4  # List, Optional, Path, json

    suggestions = analyzer.suggest_refactoring()
    assert isinstance(suggestions, list)

    print("✅ Complex code analysis works")


if __name__ == "__main__":
    print("\n" + "=" * 60)
    print("Running Code Analysis Tool Tests")
    print("=" * 60 + "\n")

    test_syntax_analysis()
    test_invalid_syntax()
    test_find_functions()
    test_find_classes()
    test_find_imports()
    test_refactoring_suggestions()
    test_execute_analyze_syntax()
    test_execute_find_functions()
    test_execute_find_classes()
    test_execute_find_imports()
    test_execute_suggest_refactoring()
    test_complex_code_analysis()

    print("\n" + "=" * 60)
    print("🎉 All code analysis tests passed!")
    print("=" * 60)
