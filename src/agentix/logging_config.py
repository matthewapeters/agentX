"""
Logging configuration for Agentix with structured JSON output.

This module configures Python logging to output structured JSON logs
for programmatic analysis, particularly for classification decisions.
"""

import logging
import logging.config
import json
from datetime import datetime
from pathlib import Path


class JSONFormatter(logging.Formatter):
    """Format log records as JSON objects."""

    def format(self, record):
        """Format a log record as JSON."""
        log_obj = {
            "timestamp": datetime.utcnow().isoformat() + "Z",
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


LOGGING_CONFIG = {
    "version": 1,
    "disable_existing_loggers": False,
    "formatters": {
        "json": {
            "()": "agentix.logging_config.JSONFormatter",
        },
        "console": {"format": "%(asctime)s [%(levelname)s] %(name)s: %(message)s"},
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
