"""
Structured JSON Lines logger that mirrors everything written to the output pane.

Each line in the file is a self-contained JSON object:
  {"epoch": <float>, "source": "<source>", "text": "<text>"}

Sources:
  user              — prompt submitted by the user
  classification    — intent/reasoning from the prompt classifier
  thinking          — model reasoning / thinking tokens (full accumulated text)
  agent             — assistant response (full accumulated text)
  tool_call         — tool invocation (name + serialised input)
  tool_result       — tool output (name + serialised result)
  error             — error messages
"""

import json
import os
import time


class OutputLogger:
    """Appends structured log entries to ``output_log.jsonl`` inside the session folder."""

    def __init__(self, session_folder: str) -> None:
        self._path = os.path.join(session_folder, "output_log.jsonl")
        # line-buffered so each entry is flushed immediately
        self._file = open(self._path, "a", encoding="utf-8", buffering=1)

    # ------------------------------------------------------------------
    # Public API
    # ------------------------------------------------------------------

    def log(self, source: str, text: str) -> None:
        """Append a single entry. No-op if *text* is empty."""
        if not text:
            return
        entry = {
            "epoch": time.time(),
            "source": source,
            "text": text,
        }
        try:
            self._file.write(json.dumps(entry, ensure_ascii=False) + "\n")
        except Exception:
            pass

    def close(self) -> None:
        try:
            self._file.close()
        except Exception:
            pass

    def __del__(self) -> None:
        self.close()
