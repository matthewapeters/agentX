"""agentix.tools.ast_tools.py"""

import ast
from glob import glob


def module_files(root_path: str, module_prefix: str) -> list:
    """Get all Python files in a module directory."""
    return [
        f"{root_path}/{f}"
        for f in glob("**/*.py", root_dir=root_path, recursive=True)
        if f.startswith(module_prefix)
    ]


def ast_tree(file_path) -> ast.Module:
    """Parse a Python file into an AST."""
    source = ""
    with open(file_path, "r", encoding="utf8") as f:
        source = f.read()
    return ast.parse(source, filename=file_path, mode="exec")


def ast_module(module_files_list: list) -> dict[str, ast.Module]:
    """Parse multiple Python files into ASTs."""
    asts = {}
    for file_path in module_files_list:
        asts[file_path] = ast_tree(file_path)
    return asts


def node_to_dict(node):
    """node_to_dict converts an AST node into a dictionary representation."""
    if isinstance(node, ast.AST):
        return {
            "_type": node.__class__.__name__,
            **{k: node_to_dict(v) for k, v in ast.iter_fields(node)},
            **{
                k: getattr(node, k)
                for k in ("lineno", "col_offset", "end_lineno", "end_col_offset")
                if hasattr(node, k)
            },
        }
    elif isinstance(node, list):
        return [node_to_dict(n) for n in node]
    else:
        return node  # constants, strings, etc.


def class_implements(ast_node: ast.ClassDef, method_names: list[str]) -> bool:
    """Check if a class AST node implements a method with the given name."""
    checks = []
    for item in ast_node.body:
        if isinstance(item, ast.FunctionDef) and item.name in method_names:
            checks.append(True)
    return len(checks) == len(method_names)


def module_classes_implementing(
    module: ast.Module, method_names: list[str]
) -> list[ast.ClassDef]:
    """Check if a module AST has a class implementing the given methods."""
    return [
        node
        for node in module.body
        if isinstance(node, ast.ClassDef) and class_implements(node, method_names)
    ]


def extract_function_defs_from_class_node(
    class_node: ast.ClassDef, function_names: list[str]
) -> dict[str, ast.FunctionDef]:
    """Extract all function definitions from a class AST class node."""
    return {
        node.name: {
            "node": node,
            "args": node.args,
            "returns": node.returns,
            "source": ast.unparse(node),
        }
        for node in class_node.body
        if isinstance(node, ast.FunctionDef) and node.name in function_names
    }
