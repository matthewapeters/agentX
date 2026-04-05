"""
Logging configuration for Agentix with structured JSON output.

This module configures Python logging to output structured JSON logs
for programmatic analysis, particularly for classification decisions.
"""

import logging
import logging.config
import json
from datetime import datetime, timezone
from pathlib import Path


class JSONFormatter(logging.Formatter):
    """Format log records as JSON objects."""

    def format(self, record):
        """Format a log record as JSON."""
        log_obj = {
            "timestamp": datetime.now(timezone.utc).isoformat(),
            "level": record.levelname,
            "logger": record.name,
            "message": record.getMessage(),
        }

        # Add any extra fields from the log record
        if hasattr(record, "__dict__"):
            for key, value in record.__dict__.items():
                if key not in (
                    "name",
                    "msg",
                    "args",
                    "created",
                    "filename",
                    "funcName",
                    "levelname",
                    "levelno",
                    "lineno",
                    "module",
                    "msecs",
                    "pathname",
                    "process",
                    "processName",
                    "relativeCreated",
                    "thread",
                    "threadName",
                    "exc_info",
                    "exc_text",
                    "stack_info",
                    "getMessage",
                    "message",
                ):
                    log_obj[key] = value

        # Add exception info if present
        if record.exc_info:
            log_obj["exception"] = self.formatException(record.exc_info)

        return json.dumps(log_obj)


class DetailedFormatter(logging.Formatter):
    """
    Format log records with extra fields displayed in human-readable format.

    This formatter automatically displays all extra fields from structured logging,
    which is critical for debugging JSON parsing errors where we need to see the
    actual raw strings that failed.
    """

    def format(self, record):
        """Format a log record with extra fields displayed."""
        # Standard formatting
        base_message = super().format(record)

        # Collect extra fields (anything not in standard log record attributes)
        extra_fields = []
        standard_attrs = {
            "name",
            "msg",
            "args",
            "created",
            "filename",
            "funcName",
            "levelname",
            "levelno",
            "lineno",
            "module",
            "msecs",
            "message",
            "pathname",
            "process",
            "processName",
            "relativeCreated",
            "thread",
            "threadName",
            "exc_info",
            "exc_text",
            "stack_info",
            "asctime",
            "taskName",
        }

        for key, value in record.__dict__.items():
            if key not in standard_attrs:
                # Special handling for raw content - show repr() and truncate smartly
                if any(keyword in key.lower() for keyword in ["raw", "content", "payload", "answer"]):
                    value_str = repr(value) if isinstance(value, str) else str(value)
                    # For very long strings, show beginning and end
                    if len(value_str) > 500:
                        extra_fields.append(f"\n    {key} (first 250 chars): {value_str[:250]}")
                        extra_fields.append(f"\n    {key} (last 250 chars): {value_str[-250:]}")
                        extra_fields.append(f"\n    {key} (total length): {len(value_str)} chars")
                    else:
                        extra_fields.append(f"\n    {key}: {value_str}")
                else:
                    extra_fields.append(f"\n    {key}: {value}")

        if extra_fields:
            base_message += "".join(extra_fields)

        return base_message


LOGGING_CONFIG = {
    "version": 1,
    "disable_existing_loggers": False,
    "formatters": {
        "json": {
            "()": "agentix.logging_config.JSONFormatter",
        },
        "console": {
            "()": "agentix.logging_config.DetailedFormatter",
            "format": "%(asctime)s [%(levelname)s] %(name)s: %(message)s",
            "datefmt": "%Y-%m-%d %H:%M:%S",
        },
        "simple": {"format": "%(asctime)s [%(levelname)s] %(name)s: %(message)s"},
    },
    "handlers": {
        "console": {
            "class": "logging.StreamHandler",
            "formatter": "console",
            "stream": "ext://sys.stderr",
        },
        "classification_file": {
            "class": "logging.handlers.RotatingFileHandler",
            "filename": "logs/classification.jsonl",
            "formatter": "json",
            "maxBytes": 10485760,  # 10MB
            "backupCount": 5,
        },
    },
    "loggers": {
        "agentix.classification": {
            "handlers": ["console", "classification_file"],
            "level": "INFO",
            "propagate": False,
        },
        "agentx.adapter": {
            "handlers": ["console"],
            "level": "INFO",
            "propagate": False,
        },
    },
    "root": {
        "handlers": ["console"],
        "level": "WARNING",
    },
}


def configure_logging(log_dir: str = "logs"):
    """
    Configure logging with structured JSON output.

    Args:
        log_dir: Directory for log files (default: "logs")
    """
    # Ensure log directory exists
    Path(log_dir).mkdir(parents=True, exist_ok=True)

    # Update file handler path if custom log_dir provided
    if log_dir != "logs":
        LOGGING_CONFIG["handlers"]["classification_file"]["filename"] = f"{log_dir}/classification.jsonl"

    logging.config.dictConfig(LOGGING_CONFIG)
