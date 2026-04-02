"""Client for communicating with the URL Shortener API server."""

import requests


class APIClient:
    def __init__(self, base_url: str = "http://localhost:8080"):
        self.base_url = base_url.rstrip("/")

    def shorten(self, url: str) -> dict:
        resp = requests.post(
            f"{self.base_url}/api/shorten",
            json={"url": url},
            timeout=10,
        )
        resp.raise_for_status()
        return resp.json()

    def get_stats(self) -> dict:
        resp = requests.get(f"{self.base_url}/api/stats", timeout=10)
        resp.raise_for_status()
        return resp.json()

    def get_url_stats(self, short_code: str) -> dict:
        resp = requests.get(
            f"{self.base_url}/api/stats/{short_code}", timeout=10
        )
        resp.raise_for_status()
        return resp.json()
