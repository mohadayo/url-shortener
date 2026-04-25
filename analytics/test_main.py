"""Tests for the analytics CLI main module."""

import unittest
from unittest.mock import patch, MagicMock
from io import StringIO

from client import APIError


SAMPLE_STATS = {
    "total_urls": 3,
    "total_clicks": 50,
    "entries": [
        {
            "original_url": "https://example.com/page1",
            "short_code": "abc12345",
            "created_at": "2025-01-15T10:00:00Z",
            "clicks": 30,
        },
        {
            "original_url": "https://github.com/repo",
            "short_code": "def67890",
            "created_at": "2025-01-16T09:00:00Z",
            "clicks": 15,
        },
        {
            "original_url": "https://docs.python.org/3/",
            "short_code": "ghi11111",
            "created_at": "2025-01-16T12:00:00Z",
            "clicks": 5,
        },
    ],
}

SAMPLE_ENTRY = {
    "short_code": "abc12345",
    "original_url": "https://example.com/page1",
    "clicks": 30,
    "created_at": "2025-01-15T10:00:00Z",
}


class TestReportCommand(unittest.TestCase):
    """report コマンドのテスト"""

    @patch("main.APIClient")
    def test_report_prints_output(self, MockClient):
        mock_client = MagicMock()
        mock_client.get_stats.return_value = SAMPLE_STATS
        MockClient.return_value = mock_client

        with patch("sys.argv", ["main", "report"]):
            with patch("sys.stdout", new_callable=StringIO) as mock_stdout:
                import main
                main.main()
                output = mock_stdout.getvalue()
                self.assertIn("Total URLs:   3", output)
                self.assertIn("Total Clicks: 50", output)

    @patch("main.APIClient")
    def test_report_with_custom_api_url(self, MockClient):
        mock_client = MagicMock()
        mock_client.get_stats.return_value = {
            "total_urls": 0, "total_clicks": 0, "entries": []
        }
        MockClient.return_value = mock_client

        with patch("sys.argv", ["main", "--api-url", "http://custom:9090", "report"]):
            with patch("sys.stdout", new_callable=StringIO):
                import main
                main.main()
                MockClient.assert_called_with("http://custom:9090")


class TestTopCommand(unittest.TestCase):
    """top コマンドのテスト"""

    @patch("main.APIClient")
    def test_top_prints_entries(self, MockClient):
        mock_client = MagicMock()
        mock_client.get_stats.return_value = SAMPLE_STATS
        MockClient.return_value = mock_client

        with patch("sys.argv", ["main", "top"]):
            with patch("sys.stdout", new_callable=StringIO) as mock_stdout:
                import main
                main.main()
                output = mock_stdout.getvalue()
                self.assertIn("abc12345", output)
                self.assertIn("30", output)


class TestDomainsCommand(unittest.TestCase):
    """domains コマンドのテスト"""

    @patch("main.APIClient")
    def test_domains_prints_domains(self, MockClient):
        mock_client = MagicMock()
        mock_client.get_stats.return_value = SAMPLE_STATS
        MockClient.return_value = mock_client

        with patch("sys.argv", ["main", "domains"]):
            with patch("sys.stdout", new_callable=StringIO) as mock_stdout:
                import main
                main.main()
                output = mock_stdout.getvalue()
                self.assertIn("example.com", output)
                self.assertIn("github.com", output)


class TestLookupCommand(unittest.TestCase):
    """lookup コマンドのテスト"""

    @patch("main.APIClient")
    def test_lookup_prints_entry(self, MockClient):
        mock_client = MagicMock()
        mock_client.get_url_stats.return_value = SAMPLE_ENTRY
        MockClient.return_value = mock_client

        with patch("sys.argv", ["main", "lookup", "abc12345"]):
            with patch("sys.stdout", new_callable=StringIO) as mock_stdout:
                import main
                main.main()
                output = mock_stdout.getvalue()
                self.assertIn("abc12345", output)
                self.assertIn("https://example.com/page1", output)
                self.assertIn("30", output)


class TestErrorHandling(unittest.TestCase):
    """エラーハンドリングのテスト"""

    @patch("main.APIClient")
    def test_api_error_prints_error_and_exits(self, MockClient):
        mock_client = MagicMock()
        mock_client.get_stats.side_effect = APIError("接続エラー")
        MockClient.return_value = mock_client

        with patch("sys.argv", ["main", "report"]):
            with patch("sys.stderr", new_callable=StringIO) as mock_stderr:
                import main
                with self.assertRaises(SystemExit) as ctx:
                    main.main()
                self.assertEqual(ctx.exception.code, 1)
                self.assertIn("接続エラー", mock_stderr.getvalue())

    @patch("main.APIClient")
    def test_no_command_prints_help_and_exits(self, MockClient):
        with patch("sys.argv", ["main"]):
            import main
            with self.assertRaises(SystemExit) as ctx:
                main.main()
            self.assertEqual(ctx.exception.code, 1)


if __name__ == "__main__":
    unittest.main()
