"""TUI output bridge for writing streamed chat records to a FIFO."""

from __future__ import annotations

import errno
import logging
import os
import select
import threading
import time
from collections.abc import Callable

logger = logging.getLogger(__name__)

SUBMIT_SENTINEL = "\n---SUBMIT---\n"


class TuiBridge:
    """Bridge that writes chat stream records and reads TUI submit input.

    The writer is intentionally non-blocking: if there is no FIFO reader, the
    write is dropped so GUI streaming latency is never impacted.
    """

    def __init__(
        self,
        output_fifo: str,
        input_fifo: str | None = None,
        on_submit: Callable[[str], None] | None = None,
        enabled: bool = True,
        write_timeout_sec: float = 0.1,
    ) -> None:
        """Initialize bridge configuration.

        Args:
            output_fifo: FIFO path used for output records.
            input_fifo: Optional FIFO path used for submit input records.
            on_submit: Callback invoked with submitted text.
            enabled: Whether output writes are enabled.
            write_timeout_sec: Maximum time to wait for writable FIFO state.
        """
        self.output_fifo = output_fifo
        self.input_fifo = input_fifo or ""
        self._on_submit = on_submit
        self._enabled = bool(enabled)
        self.write_timeout_sec = max(0.0, float(write_timeout_sec))
        self._started = False
        self._stop_event = threading.Event()
        self._input_thread: threading.Thread | None = None

    @property
    def is_enabled(self) -> bool:
        """Return True when the bridge should attempt writes."""
        return self._enabled and self._started

    def start(self) -> None:
        """Enable runtime writes for this bridge."""
        self._stop_event.clear()
        self._started = True
        if self.input_fifo and self._on_submit is not None and self._input_thread is None:
            self._input_thread = threading.Thread(target=self._input_reader_loop, daemon=True)
            self._input_thread.start()

    def stop(self) -> None:
        """Disable runtime writes for this bridge."""
        self._started = False
        self._stop_event.set()
        if self._input_thread is not None and self._input_thread.is_alive():
            self._input_thread.join(timeout=1.0)
        self._input_thread = None

    def _input_reader_loop(self) -> None:
        """Read submit messages from input FIFO and dispatch callbacks."""
        assert self._on_submit is not None
        fd: int | None = None
        buffer = ""

        try:
            while not self._stop_event.is_set():
                if fd is None:
                    try:
                        fd = os.open(self.input_fifo, os.O_RDONLY | os.O_NONBLOCK)
                    except FileNotFoundError:
                        logger.debug("TUI input fifo not found: %s", self.input_fifo)
                        time.sleep(0.1)
                        continue
                    except OSError as exc:
                        if exc.errno in {errno.ENOENT, errno.ENXIO, errno.EAGAIN, errno.EWOULDBLOCK}:
                            logger.debug("TUI input fifo unavailable: %s", exc)
                            time.sleep(0.1)
                            continue
                        logger.debug("TUI input open failed: %s", exc)
                        time.sleep(0.1)
                        continue

                readable, _, _ = select.select([fd], [], [], 0.1)
                if not readable:
                    continue

                chunk = os.read(fd, 4096)
                if not chunk:
                    # Keep the FIFO reader fd open across writer disconnects.
                    # Closing/reopening introduces readerless gaps that can block
                    # TUI submit opens on the writer side.
                    time.sleep(0.05)
                    continue

                buffer += chunk.decode("utf-8", errors="replace")
                while SUBMIT_SENTINEL in buffer:
                    raw_text, buffer = buffer.split(SUBMIT_SENTINEL, 1)
                    submitted = raw_text.strip()
                    if not submitted:
                        continue
                    try:
                        self._on_submit(submitted)
                    except Exception as exc:
                        logger.debug("TUI submit callback failed: %s", exc)
        finally:
            if fd is not None:
                try:
                    os.close(fd)
                except OSError:
                    pass

    def write_output(self, record: str) -> bool:
        """Write one output record to the FIFO without blocking the GUI path.

        Args:
            record: Output record text to write.

        Returns:
            True when the record is fully written, else False.
        """
        if not self.is_enabled:
            return False
        if not record:
            return False

        fd: int | None = None
        try:
            fd = os.open(self.output_fifo, os.O_WRONLY | os.O_NONBLOCK)

            _, writable, _ = select.select([], [fd], [], self.write_timeout_sec)
            if not writable:
                logger.debug("TUI output write timed out for fifo=%s", self.output_fifo)
                return False

            payload = record.encode("utf-8", errors="replace")
            offset = 0
            while offset < len(payload):
                written = os.write(fd, payload[offset:])
                if written <= 0:
                    logger.debug("TUI output write returned non-positive length for fifo=%s", self.output_fifo)
                    return False
                offset += written
            return True

        except FileNotFoundError:
            logger.debug("TUI output fifo not found: %s", self.output_fifo)
            return False
        except OSError as exc:
            if exc.errno in {errno.ENXIO, errno.EPIPE, errno.ENOENT, errno.EAGAIN, errno.EWOULDBLOCK}:
                logger.debug("TUI output dropped (fifo unavailable): %s", exc)
                return False
            logger.debug("TUI output write failed: %s", exc)
            return False
        finally:
            if fd is not None:
                try:
                    os.close(fd)
                except OSError:
                    pass
