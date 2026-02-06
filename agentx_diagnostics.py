#!/usr/bin/env python
"""
AgentX Service Diagnostic Tool

Run this script to verify that Ollama and Agentix are properly configured and running.
Usage: python agentx_diagnostics.py
"""

import sys
import json
from pathlib import Path

# Add src to path
sys.path.insert(0, str(Path(__file__).parent / "src"))

from agentx.config import load_config
from agentx.service_manager import ServiceManager


def print_header(text):
    """Print a formatted header."""
    print("\n" + "=" * 70)
    print(f"  {text}")
    print("=" * 70)


def check_ollama():
    """Check Ollama service and models."""
    import httpx
    
    print_header("OLLAMA CHECK")
    
    config = load_config()
    ollama_host = config["agentx"]["ollama_host"]
    
    print(f"\n📍 Ollama Host: {ollama_host}")
    
    # Check health
    try:
        with httpx.Client(timeout=5) as client:
            # Try /api/tags first (more reliable)
            response = client.get(f"http://{ollama_host}/api/tags")
            if response.status_code == 200:
                print("✅ Ollama is running and responding")
                
                # Get models
                data = response.json()
                models = data.get("models", [])
                
                if models:
                    print(f"\n📦 Available Models ({len(models)}):")
                    for model in models:
                        name = model.get("name", "unknown")
                        size = model.get("size", 0)
                        size_gb = size / (1024**3)
                        modified = model.get("modified_at", "unknown")
                        print(f"  • {name:<40} ({size_gb:>6.2f} GB)")
                else:
                    print("\n⚠️  No models available in Ollama")
                    print("   Try: ollama pull gpt-oss")
                
                return True
            else:
                print(f"❌ Ollama returned status {response.status_code}")
                return False
    except Exception as e:
        print(f"❌ Cannot connect to Ollama: {e}")
        print("   Make sure Ollama is running: ollama serve")
        return False


def check_agentix():
    """Check Agentix service."""
    import httpx
    
    print_header("AGENTIX CHECK")
    
    config = load_config()
    agentix_enabled = config.get("agentix", {}).get("enabled", False)
    agentix_host = config.get("agentix", {}).get("host", "localhost:8000")
    
    print(f"\n🔧 Agentix Configuration:")
    print(f"   Enabled: {agentix_enabled}")
    print(f"   Host: {agentix_host}")
    
    if not agentix_enabled:
        print("\n✅ Agentix is disabled (optional)")
        print("   To enable code analysis tools, set 'enabled = true' in agentx.toml")
        return None
    
    # Check if running
    try:
        with httpx.Client(timeout=5) as client:
            # Try the models endpoint which exists on Agentix
            response = client.get(f"http://{agentix_host}/v1/models")
            if response.status_code == 200:
                print(f"\n✅ Agentix is running and responding")
                return True
            else:
                print(f"\n❌ Agentix returned status {response.status_code}")
                return False
    except Exception as e:
        print(f"\n❌ Cannot connect to Agentix: {e}")
        print("   Make sure Agentix is installed: pip install agentix")
        print("   And dependencies: pip install libcst")
        return False


def check_dependencies():
    """Check required Python dependencies."""
    print_header("DEPENDENCY CHECK")
    
    dependencies = {
        "httpx": "HTTP client",
        "ollama": "Ollama client",
        "toml": "TOML config parser",
        "libcst": "Code syntax library (optional)",
        "agentix": "Agentix bridge (optional)",
    }
    
    print("\n📦 Checking dependencies:\n")
    
    for package, description in dependencies.items():
        try:
            __import__(package)
            optional = " (optional)" in description
            symbol = "✅" if not optional else "✅ "
            print(f"{symbol} {package:<15} - {description}")
        except ImportError:
            optional = "(optional)" in description
            if optional:
                print(f"⚠️  {package:<15} - {description} - NOT INSTALLED")
            else:
                print(f"❌ {package:<15} - {description} - MISSING")


def main():
    """Run all diagnostics."""
    print("\n")
    print("╔" + "=" * 68 + "╗")
    print("║" + " " * 15 + "AgentX SERVICE DIAGNOSTICS" + " " * 27 + "║")
    print("╚" + "=" * 68 + "╝")
    
    # Check dependencies
    check_dependencies()
    
    # Check Ollama
    ollama_ok = check_ollama()
    
    # Check Agentix
    agentix_status = check_agentix()
    
    # Summary
    print_header("SUMMARY")
    print()
    
    if ollama_ok:
        print("✅ Ollama is ready for use")
    else:
        print("❌ Ollama is not available - startup will fail")
        print("   Start Ollama: ollama serve")
    
    if agentix_status is None:
        print("ℹ️  Agentix is disabled (basic tools only)")
    elif agentix_status:
        print("✅ Agentix is ready - code analysis available")
    else:
        print("⚠️  Agentix startup failed - code analysis unavailable")
        print("   Install dependencies: pip install libcst agentix")
    
    print("\n" + "=" * 70 + "\n")
    
    # Recommend next step
    if ollama_ok:
        print("🚀 Ready to run: python -m agentx\n")
    else:
        print("⚠️  Fix Ollama and retry\n")


if __name__ == "__main__":
    try:
        main()
    except KeyboardInterrupt:
        print("\n\nDiagnostics cancelled")
        sys.exit(0)
    except Exception as e:
        print(f"\n\n❌ Error during diagnostics: {e}\n")
        sys.exit(1)
