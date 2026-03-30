"""Startup guard tests for agentx.main."""

import unittest
from unittest.mock import patch

from agentx.main import main


class TestMainStartup(unittest.TestCase):
    """Verify root-user startup behavior."""

    @patch("agentx.main.AgentXSession")
    @patch("agentx.main._configure_logging")
    @patch("builtins.print")
    @patch("agentx.main.os.geteuid", return_value=0)
    @patch("agentx.main.os.getenv")
    def test_main_exits_when_run_as_root(
        self,
        mock_getenv,
        mock_geteuid,
        mock_print,
        mock_configure_logging,
        mock_session,
    ):
        """Main should print an error and exit when the current user is root."""
        mock_getenv.side_effect = lambda key, default=None: {
            "USER": "root",
            "USERNAME": "root",
        }.get(key, default)

        with self.assertRaises(SystemExit) as exc:
            main()

        self.assertEqual(exc.exception.code, 1)
        mock_print.assert_called_once()
        mock_configure_logging.assert_not_called()
        mock_session.assert_not_called()
