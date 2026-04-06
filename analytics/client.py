"""Client for communicating with the URL Shortener API server."""

import requests
from requests.exceptions import ConnectionError, Timeout, RequestException


class APIError(Exception):
    """APIサーバーとの通信エラーを表す例外"""

    def __init__(self, message: str, status_code: int | None = None):
        super().__init__(message)
        self.status_code = status_code


class APIClient:
    def __init__(self, base_url: str = "http://localhost:8080"):
        self.base_url = base_url.rstrip("/")

    def shorten(self, url: str) -> dict:
        try:
            resp = requests.post(
                f"{self.base_url}/api/shorten",
                json={"url": url},
                timeout=10,
            )
            resp.raise_for_status()
            return resp.json()
        except ConnectionError:
            raise APIError(
                f"APIサーバーに接続できません: {self.base_url}\nサーバーが起動しているか確認してください。"
            )
        except Timeout:
            raise APIError("APIサーバーへのリクエストがタイムアウトしました。")
        except requests.HTTPError as e:
            try:
                detail = e.response.json().get("error", str(e))
            except Exception:
                detail = str(e)
            raise APIError(f"APIエラー: {detail}", status_code=e.response.status_code)
        except RequestException as e:
            raise APIError(f"ネットワークエラー: {e}")

    def get_stats(self) -> dict:
        try:
            resp = requests.get(f"{self.base_url}/api/stats", timeout=10)
            resp.raise_for_status()
            return resp.json()
        except ConnectionError:
            raise APIError(
                f"APIサーバーに接続できません: {self.base_url}\nサーバーが起動しているか確認してください。"
            )
        except Timeout:
            raise APIError("APIサーバーへのリクエストがタイムアウトしました。")
        except requests.HTTPError as e:
            try:
                detail = e.response.json().get("error", str(e))
            except Exception:
                detail = str(e)
            raise APIError(f"APIエラー: {detail}", status_code=e.response.status_code)
        except RequestException as e:
            raise APIError(f"ネットワークエラー: {e}")

    def get_url_stats(self, short_code: str) -> dict:
        try:
            resp = requests.get(
                f"{self.base_url}/api/stats/{short_code}", timeout=10
            )
            resp.raise_for_status()
            return resp.json()
        except ConnectionError:
            raise APIError(
                f"APIサーバーに接続できません: {self.base_url}\nサーバーが起動しているか確認してください。"
            )
        except Timeout:
            raise APIError("APIサーバーへのリクエストがタイムアウトしました。")
        except requests.HTTPError as e:
            try:
                detail = e.response.json().get("error", str(e))
            except Exception:
                detail = str(e)
            raise APIError(f"APIエラー: {detail}", status_code=e.response.status_code)
        except RequestException as e:
            raise APIError(f"ネットワークエラー: {e}")
