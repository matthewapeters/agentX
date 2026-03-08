"""
Docstring for agentx.main
"""

import logging
import tkinter as tk
from pathlib import Path

from .config import load_config
from .session import AgentXSession


def _configure_logging() -> None:
    """Set up application-wide logging to both stderr and a rolling log file."""
    log_dir = Path(__file__).parent.parent.parent / "sessions" / "_logs"
    log_dir.mkdir(parents=True, exist_ok=True)
    log_file = log_dir / "agentx.log"

    fmt = "%(asctime)s [%(levelname)s] %(name)s: %(message)s"
    logging.basicConfig(
        level=logging.DEBUG,
        format=fmt,
        handlers=[
            logging.FileHandler(log_file, encoding="utf-8"),
            logging.StreamHandler(),
        ],
    )
    # Silence noisy third-party loggers.
    for noisy in ("urllib3", "httpx", "httpcore", "asyncio"):
        logging.getLogger(noisy).setLevel(logging.WARNING)


def main():
    """
    Docstring for main
    """
    _configure_logging()
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
