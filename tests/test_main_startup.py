"""Startup guard tests for agentx.main."""

import runpy
import unittest
from unittest.mock import patch

from agentx.main import main


class TestMainStartup(unittest.TestCase):
    """Verify root-user startup behavior."""

    @patch("agentx.main.AgentXSession")
    @patch("agentx.main._configure_logging")
    @patch("agentx.main.os.geteuid", return_value=0)
    @patch("agentx.main.os.getenv")
    def test_main_exits_when_run_as_root(
        self,
        mock_getenv,
        mock_geteuid,
        mock_configure_logging,
        mock_session,
    ):
        """Main should log an error and exit when the current user is root."""
        mock_getenv.side_effect = lambda key, default=None: {
            "USER": "root",
            "USERNAME": "root",
        }.get(key, default)

        with self.assertLogs("agentx.main", level="ERROR") as log_ctx:
            with self.assertRaises(SystemExit) as exc:
                main()

        self.assertEqual(exc.exception.code, 1)
        self.assertTrue(any("root" in m.lower() for m in log_ctx.output))
        mock_configure_logging.assert_not_called()
        mock_session.assert_not_called()

    def test_main_module_entry_point(self):
        """__main__.py runs main() when invoked as python -m agentx."""
        with patch("agentx.main.main") as mock_main:
            runpy.run_module("agentx", run_name="__main__", alter_sys=False)
        mock_main.assert_called_once()
