# agentix/tools/cst_tools.py

from __future__ import annotations

from glob import glob
from typing import Dict, List, Optional

import libcst as cst
from libcst.metadata import CodeRange, MetadataWrapper, PositionProvider

# --------------------------------------------------------------------------------------
# Filesystem helpers (kept compatible with your original behavior)
# --------------------------------------------------------------------------------------


def module_files(root_path: str, module_prefix: str) -> List[str]:
    """
    Get all Python files in a module directory (recursive) whose path (relative
    to root_path) starts with `module_prefix`.

    This mirrors:
        [f"{root_path}/{f}" for f in glob("**/*.py", root_dir=root_path, recursive=True)
         if f.startswith(module_prefix)]
    """
    return [
        f"{root_path}/{rel}"
        for rel in glob("**/*.py", root_dir=root_path, recursive=True)
        if rel.startswith(module_prefix)
    ]


# --------------------------------------------------------------------------------------
# Parsing helpers (LibCST)
# --------------------------------------------------------------------------------------


def cst_tree(file_path: str) -> cst.Module:
    """Parse a Python file into a LibCST Module (Concrete Syntax Tree)."""
    with open(file_path, "r", encoding="utf8") as f:
        source = f.read()
    return cst.parse_module(source)


def cst_modules(module_files_list: List[str]) -> Dict[str, cst.Module]:
    """Parse multiple Python files into LibCST Modules."""
    return {file_path: cst_tree(file_path) for file_path in module_files_list}


# --------------------------------------------------------------------------------------
# Metadata & serialization helpers
# --------------------------------------------------------------------------------------


def with_metadata(module: cst.Module) -> MetadataWrapper:
    """
    Wrap a module with metadata so you can resolve node positions, scopes, etc.
    Typical usage:
        mod = cst.parse_module(src)
        wrapper = with_metadata(mod)
        pos = node_positions(wrapper, some_node)
    """
    return MetadataWrapper(module)


def node_positions(wrapper: MetadataWrapper, node: cst.CSTNode) -> Dict[str, int]:
    """
    Return 1-based line/column positions for a node using PositionProvider metadata:
        {
            "lineno": ...,
            "col_offset": ...,
            "end_lineno": ...,
            "end_col_offset": ...
        }
    """
    rng: CodeRange = wrapper.resolve(PositionProvider, node)
    return {
        "lineno": rng.start.line,
        "col_offset": rng.start.column,
        "end_lineno": rng.end.line,
        "end_col_offset": rng.end.column,
    }


def cst_node_to_dict(
    wrapper: MetadataWrapper, node: cst.CSTNode, include_source: bool = True
) -> Dict:
    """
    Convert a CST node into a shallow, JSON-friendly dict with:
      - the node type,
      - positional info (if available),
      - optional source code for the node.

    NOTE:
      LibCST nodes preserve formatting & comments but do not expose a simple
      "fields to dict" like `ast.iter_fields`. For many tooling tasks, type,
      positions, and (optionally) code are sufficient. If you need a deep
      structural export, consider writing a specialized serializer that walks
      node.attributes explicitly.

    Returns e.g.:
        {
          "_type": "FunctionDef",
          "lineno": 3, "col_offset": 0, "end_lineno": 6, "end_col_offset": 0,
          "source": "def f(x):\\n    return x"
        }
    """
    data = {"_type": node.__class__.__name__}

    try:
        data.update(node_positions(wrapper, node))
    except Exception:
        # PositionProvider might not be attached or node may be synthetic
        pass

    if include_source:
        try:
            # `MetadataWrapper` exposes the underlying module via `.module`
            data["source"] = wrapper.module.code_for_node(node)
        except Exception:
            pass

    return data


# --------------------------------------------------------------------------------------
# Structural queries (mirroring your AST utilities)
# --------------------------------------------------------------------------------------


def class_implements(class_node: cst.ClassDef, method_names: List[str]) -> bool:
    """
    Check if a ClassDef implements *all* given method names at its immediate body level.

    NOTE:
      - `class_node.body` is an `IndentedBlock`.
      - We inspect `class_node.body.body` for `cst.FunctionDef` statements.
      - This mirrors your original behavior (top-level in the class only).
    """
    wanted = set(method_names)
    found: set[str] = set()

    # class_node.body is an IndentedBlock; .body is a list[BaseStatement]
    for stmt in class_node.body.body:
        if isinstance(stmt, cst.FunctionDef) and stmt.name.value in wanted:
            found.add(stmt.name.value)

    return wanted.issubset(found)


def module_classes_implementing(
    module: cst.Module, method_names: List[str]
) -> List[cst.ClassDef]:
    """
    Return top-level classes in the module that implement all given methods.

    NOTE:
      This matches your original approach which only checked top-level statements
      (`module.body`) rather than a deep traversal.
    """
    matches: List[cst.ClassDef] = []
    for stmt in module.body:
        if isinstance(stmt, cst.ClassDef) and class_implements(stmt, method_names):
            matches.append(stmt)
    return matches


def extract_function_defs_from_class_node(
    class_node: cst.ClassDef,
    function_names: List[str],
    *,
    module_for_code: Optional[cst.Module] = None,
) -> Dict[str, Dict]:
    """
    Extract specific FunctionDefs from a class.

    Returns a dict keyed by function name:
        {
          name: {
            "node": <cst.FunctionDef>,
            "params": <cst.Parameters>,
            "args": <cst.Parameters>,    # alias for compatibility with your API
            "returns": <Optional[cst.Annotation]>,
            "source": <str or None>
          }
        }

    If `module_for_code` is provided, we use `module_for_code.code_for_node(fn)`
    to get the exact source for each function. Otherwise, we omit `source` or
    try a best-effort generation.
    """
    want = set(function_names)
    out: Dict[str, Dict] = {}

    for stmt in class_node.body.body:
        if isinstance(stmt, cst.FunctionDef) and stmt.name.value in want:
            # In LibCST, parameters live on `params` (not `args`).
            params = stmt.params
            returns = stmt.returns  # Optional[cst.Annotation]

            # Compute source if possible.
            src = None
            if module_for_code is not None:
                try:
                    src = module_for_code.code_for_node(stmt)
                except Exception:
                    src = None
            else:
                # Best-effort: use a dummy module to render the node.
                try:
                    dummy = cst.Module(body=[])
                    src = dummy.code_for_node(stmt)
                except Exception:
                    src = None

            out[stmt.name.value] = {
                "node": stmt,
                "params": params,
                "args": params,  # alias for compatibility
                "returns": returns,
                "source": src,
            }

    return out


class AddDecorator(cst.CSTTransformer):
    """
    Adds a decorator to a specified class if it doesn't already exist.
    """

    def __init__(self, class_name: str, decorator_text: str):
        self.class_name = class_name
        self.decorator_text = decorator_text.lstrip("@")
        self.decorator_expr = cst.parse_expression(self.decorator_text)

    def leave_ClassDef(
        self, original_node: cst.ClassDef, updated_node: cst.ClassDef
    ) -> cst.ClassDef:
        """Add a decorator to the specified class if it doesn't already exist."""
        if original_node.name.value != self.class_name:
            return updated_node
        # Avoid duplicates by comparing code
        existing = [d.decorator.code for d in updated_node.decorators]
        new_code = self.decorator_expr.code
        if new_code in existing:
            return updated_node
        return updated_node.with_changes(
            decorators=[
                cst.Decorator(decorator=self.decorator_expr),
                *updated_node.decorators,
            ]
        )
