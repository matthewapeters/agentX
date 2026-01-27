"""
Docstring for agentx.main
"""

import tkinter as tk

from .config import load_config
from .layout import layout
from .ollama_client import perform_service_handshake
from .session import AgentXSession

def main():
    """
    Docstring for main
    """
    root: tk.Tk = tk.Tk()
    config = load_config()
    session = AgentXSession(root, config)

    # Perform service handshake before initializing the layout
    try:
        perform_service_handshake(config)
    except RuntimeError as e:
        print(e)
        return

    layout(session)
    root.mainloop()
