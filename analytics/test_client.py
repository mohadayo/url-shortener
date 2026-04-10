"""Tests for the API client module."""

import unittest
from unittest.mock import patch, MagicMock

from client import APIClient, APIError


class TestAPIClientInit(unittest.TestCase):
    """APIClient初期化のテスト"""

    def test_default_base_url(self):
        client = APIClient()
        self.assertEqual(client.base_url, "http://localhost:8080")

    def test_custom_base_url(self):
        client = APIClient("http://api.example.com:9090")
        self.assertEqual(client.base_url, "http://api.example.com:9090")

    def test_trailing_slash_stripped(self):
        client = APIClient("http://localhost:8080/")
        self.assertEqual(client.base_url, "http://localhost:8080")


class TestShorten(unittest.TestCase):
    """shorten メソッドのテスト"""

    @patch("client.requests.post")
    def test_successful_shorten(self, mock_post):
        mock_resp = MagicMock()
        mock_resp.json.return_value = {
            "short_code": "abc12345",
            "short_url": "http://localhost:8080/r/abc12345",
        }
        mock_resp.raise_for_status = MagicMock()
        mock_post.return_value = mock_resp

        client = APIClient()
        result = client.shorten("https://example.com")

        self.assertEqual(result["short_code"], "abc12345")
        mock_post.assert_called_once_with(
            "http://localhost:8080/api/shorten",
            json={"url": "https://example.com"},
            timeout=10,
        )

    @patch("client.requests.post")
    def test_connection_error(self, mock_post):
        from requests.exceptions import ConnectionError
        mock_post.side_effect = ConnectionError()

        client = APIClient()
        with self.assertRaises(APIError) as ctx:
            client.shorten("https://example.com")
        self.assertIn("接続できません", str(ctx.exception))

    @patch("client.requests.post")
    def test_timeout_error(self, mock_post):
        from requests.exceptions import Timeout
        mock_post.side_effect = Timeout()

        client = APIClient()
        with self.assertRaises(APIError) as ctx:
            client.shorten("https://example.com")
        self.assertIn("タイムアウト", str(ctx.exception))

    @patch("client.requests.post")
    def test_http_error_with_json_detail(self, mock_post):
        import requests
        mock_resp = MagicMock()
        mock_resp.status_code = 400
        mock_resp.json.return_value = {"error": "無効なURLです"}
        http_err = requests.HTTPError(response=mock_resp)
        mock_resp.raise_for_status.side_effect = http_err

        mock_post.return_value = mock_resp

        client = APIClient()
        with self.assertRaises(APIError) as ctx:
            client.shorten("invalid")
        self.assertIn("無効なURLです", str(ctx.exception))
        self.assertEqual(ctx.exception.status_code, 400)

    @patch("client.requests.post")
    def test_http_error_without_json(self, mock_post):
        import requests
        mock_resp = MagicMock()
        mock_resp.status_code = 500
        mock_resp.json.side_effect = ValueError("not json")
        http_err = requests.HTTPError(response=mock_resp)
        mock_resp.raise_for_status.side_effect = http_err

        mock_post.return_value = mock_resp

        client = APIClient()
        with self.assertRaises(APIError) as ctx:
            client.shorten("https://example.com")
        self.assertEqual(ctx.exception.status_code, 500)

    @patch("client.requests.post")
    def test_generic_request_exception(self, mock_post):
        from requests.exceptions import RequestException
        mock_post.side_effect = RequestException("network failure")

        client = APIClient()
        with self.assertRaises(APIError) as ctx:
            client.shorten("https://example.com")
        self.assertIn("ネットワークエラー", str(ctx.exception))


class TestGetStats(unittest.TestCase):
    """get_stats メソッドのテスト"""

    @patch("client.requests.get")
    def test_successful_get_stats(self, mock_get):
        mock_resp = MagicMock()
        mock_resp.json.return_value = {
            "total_urls": 10,
            "total_clicks": 100,
            "entries": [],
        }
        mock_resp.raise_for_status = MagicMock()
        mock_get.return_value = mock_resp

        client = APIClient()
        result = client.get_stats()

        self.assertEqual(result["total_urls"], 10)
        mock_get.assert_called_once_with(
            "http://localhost:8080/api/stats",
            timeout=10,
        )

    @patch("client.requests.get")
    def test_connection_error(self, mock_get):
        from requests.exceptions import ConnectionError
        mock_get.side_effect = ConnectionError()

        client = APIClient()
        with self.assertRaises(APIError) as ctx:
            client.get_stats()
        self.assertIn("接続できません", str(ctx.exception))

    @patch("client.requests.get")
    def test_timeout_error(self, mock_get):
        from requests.exceptions import Timeout
        mock_get.side_effect = Timeout()

        client = APIClient()
        with self.assertRaises(APIError) as ctx:
            client.get_stats()
        self.assertIn("タイムアウト", str(ctx.exception))


class TestGetUrlStats(unittest.TestCase):
    """get_url_stats メソッドのテスト"""

    @patch("client.requests.get")
    def test_successful_get_url_stats(self, mock_get):
        mock_resp = MagicMock()
        mock_resp.json.return_value = {
            "short_code": "abc12345",
            "original_url": "https://example.com",
            "clicks": 42,
        }
        mock_resp.raise_for_status = MagicMock()
        mock_get.return_value = mock_resp

        client = APIClient()
        result = client.get_url_stats("abc12345")

        self.assertEqual(result["clicks"], 42)
        mock_get.assert_called_once_with(
            "http://localhost:8080/api/stats/abc12345",
            timeout=10,
        )

    @patch("client.requests.get")
    def test_not_found_error(self, mock_get):
        import requests
        mock_resp = MagicMock()
        mock_resp.status_code = 404
        mock_resp.json.return_value = {"error": "not found"}
        http_err = requests.HTTPError(response=mock_resp)
        mock_resp.raise_for_status.side_effect = http_err

        mock_get.return_value = mock_resp

        client = APIClient()
        with self.assertRaises(APIError) as ctx:
            client.get_url_stats("nonexist")
        self.assertEqual(ctx.exception.status_code, 404)

    @patch("client.requests.get")
    def test_connection_error(self, mock_get):
        from requests.exceptions import ConnectionError
        mock_get.side_effect = ConnectionError()

        client = APIClient()
        with self.assertRaises(APIError) as ctx:
            client.get_url_stats("abc12345")
        self.assertIn("接続できません", str(ctx.exception))


class TestAPIError(unittest.TestCase):
    """APIError例外のテスト"""

    def test_error_with_status_code(self):
        err = APIError("test error", status_code=500)
        self.assertEqual(str(err), "test error")
        self.assertEqual(err.status_code, 500)

    def test_error_without_status_code(self):
        err = APIError("test error")
        self.assertEqual(str(err), "test error")
        self.assertIsNone(err.status_code)


if __name__ == "__main__":
    unittest.main()
