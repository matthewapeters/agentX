"""TUI output bridge for writing streamed chat records to a FIFO."""

from __future__ import annotations

import errno
import logging
import os
import select

logger = logging.getLogger(__name__)


class TuiBridge:
    """Bridge that writes chat stream records to a TUI output FIFO.

    The writer is intentionally non-blocking: if there is no FIFO reader, the
    write is dropped so GUI streaming latency is never impacted.
    """

    def __init__(self, output_fifo: str, enabled: bool = True, write_timeout_sec: float = 0.1) -> None:
        """Initialize bridge configuration.

        Args:
            output_fifo: FIFO path used for output records.
            enabled: Whether output writes are enabled.
            write_timeout_sec: Maximum time to wait for writable FIFO state.
        """
        self.output_fifo = output_fifo
        self._enabled = bool(enabled)
        self.write_timeout_sec = max(0.0, float(write_timeout_sec))
        self._started = False

    @property
    def is_enabled(self) -> bool:
        """Return True when the bridge should attempt writes."""
        return self._enabled and self._started

    def start(self) -> None:
        """Enable runtime writes for this bridge."""
        self._started = True

    def stop(self) -> None:
        """Disable runtime writes for this bridge."""
        self._started = False

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
