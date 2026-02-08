#!/usr/bin/env python
"""
Functional test for end-to-end chat workflow.

This test covers the full chat process:
1. User sends a message
2. Agentix classifies the prompt (if enabled)
3. System streams response
4. Output is displayed

This test can be run with or without Agentix enabled.
"""

import os
import sys
import unittest
from pathlib import Path

# Add src to path
sys.path.insert(0, str(Path(__file__).parent.parent / "src"))

from shared.models.context import Context, Message, MessageRole
from agentx.session import AgentXSession


class TestEndToEndChat(unittest.TestCase):
    """Test complete chat workflow from user input to response."""
    
    def setUp(self):
        """Set up test session."""
        # Use temp directory for test session
        import tempfile
        self.test_dir = tempfile.mkdtemp()
        
        # Configure session with Agentix disabled for predictable testing
        self.config = {
            "agentx": {
                "ollama_host": "localhost:11434",
                "ollama_model": "gpt-oss",
                "temperature": 0.7,
            },
            "agentix": {
                "enabled": False,  # Start with Agentix disabled
            }
        }
        
        self.session = AgentXSession(
            username="test_user",
            session_dir=self.test_dir,
            config=self.config
        )
    
    def tearDown(self):
        """Clean up test directory."""
        import shutil
        if os.path.exists(self.test_dir):
            shutil.rmtree(self.test_dir)
    
    def test_simple_question_without_agentix(self):
        """Test asking a simple question with Agentix disabled."""
        print("\n" + "="*60)
        print("TEST: Simple question without Agentix")
        print("="*60)
        
        prompt = "What is 2+2?"
        
        # Capture response chunks
        chunks = []
        for chunk in self.session.process_prompt(prompt):
            chunks.append(chunk)
            print(f"  [{chunk.type.name}] {chunk.content[:50] if chunk.content else ''}...")
        
        # Verify we got chunks
        self.assertGreater(len(chunks), 0, "Should receive response chunks")
        
        # Verify we got content
        content_chunks = [c for c in chunks if c.content]
        self.assertGreater(len(content_chunks), 0, "Should have content chunks")
        
        # Verify DONE chunk
        done_chunks = [c for c in chunks if c.done_reason]
        self.assertEqual(len(done_chunks), 1, "Should have exactly one DONE chunk")
        
        print(f"\n✅ Test passed: {len(chunks)} chunks received")
    
    def test_model_identification_without_agentix(self):
        """Test asking about the model without Agentix."""
        print("\n" + "="*60)
        print("TEST: Model identification without Agentix")
        print("="*60)
        
        prompt = "what model are you?"
        
        # Track chunks
        content_parts = []
        error_chunks = []
        
        for chunk in self.session.process_prompt(prompt):
            print(f"  [{chunk.type.name}]", end="")
            if chunk.content:
                print(f" {chunk.content[:50]}...", end="")
                content_parts.append(chunk.content)
            if chunk.type.name == "ERROR":
                error_chunks.append(chunk)
                print(f" ERROR: {chunk.content}")
            print()
        
        # Verify no errors
        self.assertEqual(len(error_chunks), 0, 
                        f"Should not have errors. Got: {[e.content for e in error_chunks]}")
        
        # Verify we got content
        self.assertGreater(len(content_parts), 0, "Should have content")
        
        full_response = "".join(content_parts)
        print(f"\nFull response ({len(full_response)} chars):")
        print(full_response[:200] + "..." if len(full_response) > 200 else full_response)
        print(f"\n✅ Test passed: No errors, got response")
    
    def test_simple_question_with_agentix(self):
        """Test asking a simple question with Agentix enabled."""
        # Enable Agentix
        self.config["agentix"]["enabled"] = True
        self.config["agentix"]["classify_prompts"] = True
        self.config["agentix"]["debug"] = True  # Enable debug output
        
        # Recreate session with Agentix enabled
        self.session = AgentXSession(
            username="test_user",
            session_dir=self.test_dir,
            config=self.config
        )
        
        print("\n" + "="*60)
        print("TEST: Simple question with Agentix classification")
        print("="*60)
        
        prompt = "what model are you?"
        
        # Track chunks
        chunks_by_type = {}
        errors = []
        
        for chunk in self.session.process_prompt(prompt):
            chunk_type = chunk.type.name
            if chunk_type not in chunks_by_type:
                chunks_by_type[chunk_type] = []
            chunks_by_type[chunk_type].append(chunk)
            
            print(f"  [{chunk_type}]", end="")
            if chunk.content:
                print(f" {chunk.content[:70]}...", end="")
            if chunk.classification:
                print(f" Classification: {chunk.classification}", end="")
            print()
            
            if chunk_type == "ERROR":
                errors.append(chunk.content)
        
        print(f"\nChunks by type: {list(chunks_by_type.keys())}")
        print(f"Total chunks: {sum(len(v) for v in chunks_by_type.values())}")
        
        # Verify no errors
        if errors:
            print(f"\n❌ ERRORS encountered:")
            for err in errors:
                print(f"  - {err}")
            self.fail(f"Should not have errors. Got: {errors}")
        
        # Verify we got thinking (classification)
        self.assertIn("THINKING", chunks_by_type, "Should have THINKING chunks from classification")
        
        # Verify we got content
        self.assertIn("CONTENT", chunks_by_type, "Should have CONTENT chunks")
        
        print(f"\n✅ Test passed: Agentix classification worked, got response")


if __name__ == "__main__":
    # Run with verbose output
    unittest.main(verbosity=2)
