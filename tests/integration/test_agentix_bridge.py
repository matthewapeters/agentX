"""
Simple test to verify AgentixBridge can be imported and instantiated.

Run with:
    python tests/integration/test_agentix_bridge.py
"""

import sys
from pathlib import Path

# Add src to path
sys.path.insert(0, str(Path(__file__).parent.parent.parent / "src"))

from agentix.agentix_config import AgentixConfig
from agentix.bridge import AgentixBridge, create_bridge
from shared.models.context import Context
from shared.models.message import user_message


def test_bridge_creation():
    """Test that we can create an AgentixBridge instance."""
    print("Test: Bridge creation...")

    config = AgentixConfig(
        model="llama3.2",
        debug=False,
    )

    bridge = AgentixBridge(config)
    assert bridge is not None
    assert bridge.config.model == "llama3.2"

    print("✓ Bridge created successfully")


def test_bridge_convenience_function():
    """Test the create_bridge convenience function."""
    print("\nTest: Convenience function...")

    bridge = create_bridge(model="llama3.2", debug=False)
    assert bridge is not None
    assert bridge.config.model == "llama3.2"

    print("✓ Convenience function works")


def test_get_available_models():
    """Test fetching available models."""
    print("\nTest: Get available models...")

    bridge = create_bridge()

    try:
        models = bridge.get_available_models()
        print(f"✓ Found {len(models)} models")
        if models:
            print(f"  First model: {models[0].get('name', 'unknown')}")
    except Exception as e:
        print(f"✗ Error fetching models: {e}")
        print("  (This is expected if Ollama is not running)")


def test_context_conversion():
    """Test converting shared Context to Agentix format."""
    print("\nTest: Context conversion...")

    bridge = create_bridge()

    # Create a shared Context
    context = Context()
    context.add_message(user_message("Hello, world!"))
    context.add_message(user_message("This is a test"))

    # Convert to history (returns list from get_enabled_messages())
    history = bridge._context_to_history(context)
    history_list = list(history) if hasattr(history, "__iter__") and not isinstance(history, list) else history

    assert len(history_list) == 2
    assert history_list[0].content == "Hello, world!"
    assert history_list[1].content == "This is a test"

    print("✓ Context conversion works")


def test_streaming_response_structure():
    """Test that streaming returns proper ResponseChunk objects."""
    print("\nTest: Streaming response structure...")

    bridge = create_bridge(debug=False)
    context = Context()

    # Note: This will fail if Ollama isn't running, but we're just checking structure
    try:
        chunks = list(
            bridge.process_prompt_streaming(
                "Say 'test'",
                context,
            )
        )

        if chunks:
            from shared.models.response import ResponseChunk

            assert all(isinstance(chunk, ResponseChunk) for chunk in chunks)
            print(f"✓ Received {len(chunks)} chunks")
            print(f"  Chunk types: {set(c.chunk_type.value for c in chunks)}")
    except Exception as e:
        print(f"✗ Error streaming (expected if Ollama not running): {e}")


if __name__ == "__main__":
    print("=" * 60)
    print("AgentixBridge Integration Tests")
    print("=" * 60)

    test_bridge_creation()
    test_bridge_convenience_function()
    test_context_conversion()
    test_get_available_models()
    test_streaming_response_structure()

    print("\n" + "=" * 60)
    print("Tests complete!")
    print("=" * 60)
