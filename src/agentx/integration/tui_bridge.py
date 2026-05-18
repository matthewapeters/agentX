"""TUI output bridge for writing streamed chat records to a FIFO."""

from __future__ import annotations

import errno
import logging
import os
import select
import threading
import time
from collections.abc import Callable
from dataclasses import dataclass

logger = logging.getLogger(__name__)

SUBMIT_SENTINEL = "\n---SUBMIT---\n"
QUIT_SENTINEL = "\n---QUIT---\n"

_ANSI_RESET = "\033[0m"
_WARNING_THRESHOLD = 0.80
_CRITICAL_THRESHOLD = 1.00
_DEFAULT_BAR_WIDTH = 40
_DEFAULT_TOP_WIDTH = 28


@dataclass(frozen=True)
class _ContextBand:
    """Band metadata for TUI context rendering."""

    key: str
    label: str
    emoji: str
    ascii_symbol: str
    ansi_fg: str


_CONTEXT_BANDS: tuple[_ContextBand, ...] = (
    _ContextBand(key="working_memory", label="Working Memory", emoji="💾", ascii_symbol="M", ansi_fg="96"),
    _ContextBand(key="system", label="System", emoji="🧠", ascii_symbol="S", ansi_fg="36"),
    _ContextBand(key="user", label="User", emoji="👤", ascii_symbol="U", ansi_fg="34"),
    _ContextBand(key="attachments", label="Attachments", emoji="📎", ascii_symbol="P", ansi_fg="93"),
    _ContextBand(key="thinking", label="Thinking", emoji="🤔", ascii_symbol="T", ansi_fg="35"),
    _ContextBand(key="assistant", label="Agent", emoji="🤖", ascii_symbol="A", ansi_fg="32"),
    _ContextBand(key="tool", label="Tools", emoji="🔧", ascii_symbol="L", ansi_fg="33"),
)


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
        on_quit: Callable[[], None] | None = None,
        enabled: bool = True,
        write_timeout_sec: float = 0.1,
    ) -> None:
        """Initialize bridge configuration.

        Args:
            output_fifo: FIFO path used for output records.
            input_fifo: Optional FIFO path used for submit and control records.
            on_submit: Callback invoked with submitted text.
            on_quit: Callback invoked when a quit control record is received.
            enabled: Whether output writes are enabled.
            write_timeout_sec: Maximum time to wait for writable FIFO state.
        """
        self.output_fifo = output_fifo
        self.input_fifo = input_fifo or ""
        self._on_submit = on_submit
        self._on_quit = on_quit
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
        has_input_callback = self._on_submit is not None or self._on_quit is not None
        if self.input_fifo and has_input_callback and self._input_thread is None:
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
        """Read submit and control messages from input FIFO and dispatch callbacks."""
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
                while True:
                    submit_idx = buffer.find(SUBMIT_SENTINEL)
                    quit_idx = buffer.find(QUIT_SENTINEL)
                    if submit_idx == -1 and quit_idx == -1:
                        break

                    if quit_idx != -1 and (submit_idx == -1 or quit_idx < submit_idx):
                        raw_text = buffer[:quit_idx]
                        buffer = buffer[quit_idx + len(QUIT_SENTINEL) :]
                        if raw_text.strip():
                            logger.debug("TUI quit sentinel received with ignored inline text")
                        if self._on_quit is None:
                            continue
                        try:
                            self._on_quit()
                        except Exception as exc:
                            logger.debug("TUI quit callback failed: %s", exc)
                        continue

                    raw_text = buffer[:submit_idx]
                    buffer = buffer[submit_idx + len(SUBMIT_SENTINEL) :]
                    submitted = raw_text.strip()
                    if not submitted:
                        continue
                    if self._on_submit is None:
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

    @staticmethod
    def render_context_visualization(
        max_tokens: int,
        breakdown: dict[str, int],
        *,
        use_color: bool = True,
        bar_width: int = _DEFAULT_BAR_WIDTH,
        top_width: int = _DEFAULT_TOP_WIDTH,
    ) -> str:
        """Render a context meter block for TUI output.

        Args:
            max_tokens: Context-window denominator.
            breakdown: Token counts keyed by context band.
            use_color: Whether to use ANSI color sequences for bars.
            bar_width: Character width of the main context bar.
            top_width: Character width for top contributor bars.

        Returns:
            A multiline context visualization suitable for FIFO output.
        """
        normalized_max = max(int(max_tokens), 1)
        normalized_breakdown = {k: max(int(v), 0) for k, v in breakdown.items()}
        total_tokens = sum(normalized_breakdown.values())
        usage_ratio = total_tokens / normalized_max
        usage_pct = usage_ratio * 100.0

        if usage_ratio >= _CRITICAL_THRESHOLD:
            risk = "CRIT"
        elif usage_ratio >= _WARNING_THRESHOLD:
            risk = "WARN"
        else:
            risk = "OK"

        clamped_ratio = min(max(usage_ratio, 0.0), 1.0)
        bar_slots = max(int(bar_width), 10)
        top_slots = max(int(top_width), 10)
        used_slots = int(round(bar_slots * clamped_ratio))

        slot_map = TuiBridge._allocate_slots(normalized_breakdown, normalized_max, used_slots)
        bar = TuiBridge._render_main_bar(slot_map, bar_slots, use_color=use_color)
        summary = TuiBridge._render_summary_line(normalized_breakdown, normalized_max)
        top_rows = TuiBridge._render_top_contributors(
            normalized_breakdown,
            normalized_max,
            top_slots=top_slots,
            use_color=use_color,
        )

        lines: list[str] = [
            f"###CONTEXT {usage_pct:.0f}% {risk} ({total_tokens:,} of {normalized_max:,})",
            bar,
        ]
        if summary:
            lines.append(summary)
        if top_rows:
            lines.append("Top Contributors:")
            lines.extend(top_rows)
        return "\n".join(lines) + "\n"

    @staticmethod
    def _allocate_slots(breakdown: dict[str, int], max_tokens: int, used_slots: int) -> dict[str, int]:
        """Allocate bar slots per band with largest-remainder correction."""
        if used_slots <= 0:
            return {band.key: 0 for band in _CONTEXT_BANDS}

        raw: list[tuple[str, float]] = []
        for band in _CONTEXT_BANDS:
            tokens = breakdown.get(band.key, 0)
            raw_slots = (tokens / max_tokens) * used_slots if max_tokens > 0 else 0.0
            raw.append((band.key, max(raw_slots, 0.0)))

        allocated: dict[str, int] = {key: int(val) for key, val in raw}
        remainder = used_slots - sum(allocated.values())
        if remainder > 0:
            order = sorted(raw, key=lambda item: item[1] - int(item[1]), reverse=True)
            for idx in range(remainder):
                allocated[order[idx % len(order)][0]] += 1
        elif remainder < 0:
            order = sorted(raw, key=lambda item: item[1] - int(item[1]))
            pending = abs(remainder)
            for key, _ in order:
                if pending == 0:
                    break
                if allocated[key] <= 0:
                    continue
                allocated[key] -= 1
                pending -= 1
        return allocated

    @staticmethod
    def _render_main_bar(slot_map: dict[str, int], bar_slots: int, *, use_color: bool) -> str:
        """Render main context bar with color blocks or ASCII symbols."""
        rendered: list[str] = []
        consumed = 0
        for band in _CONTEXT_BANDS:
            slots = max(slot_map.get(band.key, 0), 0)
            if slots == 0:
                continue
            consumed += slots
            if use_color:
                rendered.append(f"\033[{band.ansi_fg}m{'█' * slots}{_ANSI_RESET}")
            else:
                rendered.append(band.ascii_symbol * slots)

        remaining = max(bar_slots - consumed, 0)
        if remaining:
            if use_color:
                rendered.append(f"\033[90m{'░' * remaining}{_ANSI_RESET}")
            else:
                rendered.append("░" * remaining)
        return "".join(rendered)

    @staticmethod
    def _render_summary_line(breakdown: dict[str, int], max_tokens: int) -> str:
        """Render compact per-band percent summary."""
        parts: list[str] = []
        for band in _CONTEXT_BANDS:
            tokens = breakdown.get(band.key, 0)
            if tokens <= 0:
                continue
            pct = int(round((tokens / max_tokens) * 100)) if max_tokens > 0 else 0
            parts.append(f"{pct}% {band.label}")
        return " | ".join(parts)

    @staticmethod
    def _render_top_contributors(
        breakdown: dict[str, int],
        max_tokens: int,
        *,
        top_slots: int,
        use_color: bool,
    ) -> list[str]:
        """Render top contributor rows with color/emoji mapping."""
        band_lookup = {band.key: band for band in _CONTEXT_BANDS}
        ordered = sorted(((k, v) for k, v in breakdown.items() if v > 0), key=lambda item: item[1], reverse=True)[:4]

        rows: list[str] = []
        for index, (key, tokens) in enumerate(ordered, start=1):
            band = band_lookup.get(key)
            if band is None:
                continue
            pct = (tokens / max_tokens) * 100 if max_tokens > 0 else 0.0
            slots = max(int(round((tokens / max_tokens) * top_slots)), 1) if max_tokens > 0 else 1
            slots = min(slots, top_slots)
            if use_color:
                bar = f"\033[{band.ansi_fg}m{'█' * slots}{_ANSI_RESET}"
            else:
                bar = band.ascii_symbol * slots
            rows.append(f"  {index}. {band.emoji} {band.label:<16} {bar} {pct:.0f}%")
        return rows

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
