"""
Docstring for agentx.main
"""

import tkinter as tk

from .config import load_config
from .session import AgentXSession


def main():
    """
    Docstring for main
    """
    session = AgentXSession(tk.Tk(), load_config())

    # Perform service handshake before initializing the layout
    try:
        session.perform_service_handshake()
    except RuntimeError as e:
        print(e)
        # Cleanup services even on error
        session.service_manager.shutdown()
        return

    try:
        session.layout()
        session.root.mainloop()
    finally:
        # Cleanup services on exit
        session.service_manager.shutdown()
