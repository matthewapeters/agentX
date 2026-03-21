"""
Docstring for agentx.main
"""

import logging
import os
import sys
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


def _detect_username() -> str:
    """Return the best available username for the current process."""
    return os.getenv("USER") or os.getenv("USERNAME") or "User"


def _is_root_user(username: str) -> bool:
    """Return True when the current process is running as root."""
    if hasattr(os, "geteuid"):
        try:
            return os.geteuid() == 0
        except OSError:
            pass
    return username == "root"


def _require_non_root_user() -> str:
    """Abort startup when running as root, otherwise return the username."""
    username = _detect_username()
    if _is_root_user(username):
        print("AgentX cannot be run as root.", file=sys.stderr)
        raise SystemExit(1)
    return username


def main():
    """
    Docstring for main
    """
    username = _require_non_root_user()
    _configure_logging()
    session = AgentXSession(tk.Tk(), load_config(), username=username)

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
