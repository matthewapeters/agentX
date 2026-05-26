"""Tests for src/agentix/tools/schema.py — runtime function-to-JSON-schema conversion."""

from __future__ import annotations

import unittest
from typing import Optional

from agentix.tools.schema import SchemaGenerationError, extract_tool_schema


class TestExtractToolSchema(unittest.TestCase):

    # ------------------------------------------------------------------
    # Happy-path: simple positional arguments
    # ------------------------------------------------------------------

    def test_simple_string_args(self):
        def greet(name: str, greeting: str) -> str:
            """Return a greeting message."""
            return f"{greeting}, {name}!"

        schema = extract_tool_schema(greet)
        self.assertEqual(schema["type"], "function")
        fn = schema["function"]
        self.assertEqual(fn["name"], "greet")
        self.assertIn("Return a greeting message", fn["description"])
        props = fn["parameters"]["properties"]
        self.assertEqual(props["name"]["type"], "string")
        self.assertEqual(props["greeting"]["type"], "string")
        self.assertCountEqual(fn["parameters"]["required"], ["name", "greeting"])

    def test_numeric_types(self):
        def multiply(a: float, b: float) -> float:
            """Multiply two numbers and return the product."""
            return a * b

        schema = extract_tool_schema(multiply)
        props = schema["function"]["parameters"]["properties"]
        self.assertEqual(props["a"]["type"], "number")
        self.assertEqual(props["b"]["type"], "number")

    def test_int_and_bool_types(self):
        def repeat(text: str, times: int, uppercase: bool) -> str:
            """Repeat text a given number of times."""
            result = text * times
            return result.upper() if uppercase else result

        props = extract_tool_schema(repeat)["function"]["parameters"]["properties"]
        self.assertEqual(props["text"]["type"], "string")
        self.assertEqual(props["times"]["type"], "integer")
        self.assertEqual(props["uppercase"]["type"], "boolean")

    # ------------------------------------------------------------------
    # Optional parameters and default values
    # ------------------------------------------------------------------

    def test_optional_param_not_in_required(self):
        def search(query: str, limit: Optional[int] = None) -> list:
            """Search for items matching the query."""
            return []

        schema = extract_tool_schema(search)
        fn = schema["function"]
        required = fn["parameters"].get("required", [])
        self.assertIn("query", required)
        self.assertNotIn("limit", required)

    def test_default_value_not_in_required(self):
        def list_files(path: str, recursive: bool = False) -> list:
            """List files in a directory."""
            return []

        schema = extract_tool_schema(list_files)
        required = schema["function"]["parameters"].get("required", [])
        self.assertIn("path", required)
        self.assertNotIn("recursive", required)

    def test_all_optional_produces_no_required_key(self):
        def ping(host: str = "localhost", port: int = 80) -> bool:
            """Ping a host on a given port."""
            return True

        schema = extract_tool_schema(ping)
        # When nothing is required, the key may be omitted or empty
        required = schema["function"]["parameters"].get("required", [])
        self.assertEqual(required, [])

    # ------------------------------------------------------------------
    # Missing docstring raises SchemaGenerationError
    # ------------------------------------------------------------------

    def test_missing_docstring_raises(self):
        def no_doc(x: int) -> int:
            return x

        with self.assertRaises(SchemaGenerationError):
            extract_tool_schema(no_doc)

    def test_empty_docstring_raises(self):
        def empty_doc(x: int) -> int:
            """"""
            return x

        with self.assertRaises(SchemaGenerationError):
            extract_tool_schema(empty_doc)

    def test_whitespace_only_docstring_raises(self):
        def whitespace_doc(x: int) -> int:
            """ """
            return x

        with self.assertRaises(SchemaGenerationError):
            extract_tool_schema(whitespace_doc)

    # ------------------------------------------------------------------
    # Complex types degrade gracefully
    # ------------------------------------------------------------------

    def test_list_str_produces_array(self):
        def join_words(words: list[str], separator: str) -> str:
            """Join a list of words with the given separator."""
            return separator.join(words)

        props = extract_tool_schema(join_words)["function"]["parameters"]["properties"]
        self.assertEqual(props["words"]["type"], "array")
        self.assertEqual(props["words"]["items"]["type"], "string")

    def test_dict_produces_object(self):
        def set_metadata(key: str, values: dict[str, int]) -> None:
            """Set metadata key to the provided values."""
            pass

        props = extract_tool_schema(set_metadata)["function"]["parameters"]["properties"]
        self.assertEqual(props["values"]["type"], "object")

    def test_unannotated_param_degrades_to_string(self):
        def process(data) -> str:
            """Process arbitrary data and return a result string."""
            return str(data)

        props = extract_tool_schema(process)["function"]["parameters"]["properties"]
        self.assertEqual(props["data"]["type"], "string")

    def test_return_annotation_excluded_from_properties(self):
        def add(a: int, b: int) -> int:
            """Add two integers."""
            return a + b

        props = extract_tool_schema(add)["function"]["parameters"]["properties"]
        self.assertNotIn("return", props)

    # ------------------------------------------------------------------
    # Schema top-level structure
    # ------------------------------------------------------------------

    def test_schema_top_level_structure(self):
        def noop(x: str) -> None:
            """Do nothing with x."""
            pass

        schema = extract_tool_schema(noop)
        self.assertIn("type", schema)
        self.assertIn("function", schema)
        self.assertIn("name", schema["function"])
        self.assertIn("description", schema["function"])
        self.assertIn("parameters", schema["function"])
        self.assertEqual(schema["function"]["parameters"]["type"], "object")

    def test_docstring_is_cleaned(self):
        def verbose(x: str) -> str:
            """
            This function does something.

            It has a multi-line docstring with extra whitespace.
            """
            return x

        desc = extract_tool_schema(verbose)["function"]["description"]
        # inspect.cleandoc removes leading/trailing blank lines and normalises indent
        self.assertTrue(desc.startswith("This function does something."))


if __name__ == "__main__":
    unittest.main()
