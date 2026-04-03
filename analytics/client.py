"""Client for communicating with the URL Shortener API server."""

import logging

import requests

logger = logging.getLogger(__name__)


class APIClient:
    def __init__(self, base_url: str = "http://localhost:8080"):
        self.base_url = base_url.rstrip("/")

    def shorten(self, url: str) -> dict:
        logger.info("Shortening URL: %s", url)
        try:
            resp = requests.post(
                f"{self.base_url}/api/shorten",
                json={"url": url},
                timeout=10,
            )
            resp.raise_for_status()
            return resp.json()
        except requests.exceptions.Timeout:
            logger.error("Request timed out while shortening URL: %s", url)
            raise
        except requests.exceptions.HTTPError as exc:
            status = exc.response.status_code if exc.response is not None else None
            if status == 404:
                logger.error("API endpoint not found (404)")
            elif status and status >= 500:
                logger.error("Server error (%d) while shortening URL", status)
            else:
                logger.error("HTTP error %s while shortening URL: %s", status, exc)
            raise

    def get_stats(self) -> dict:
        logger.info("Fetching global stats")
        try:
            resp = requests.get(f"{self.base_url}/api/stats", timeout=10)
            resp.raise_for_status()
            return resp.json()
        except requests.exceptions.Timeout:
            logger.error("Request timed out while fetching stats")
            raise
        except requests.exceptions.HTTPError as exc:
            status = exc.response.status_code if exc.response is not None else None
            if status and status >= 500:
                logger.error("Server error (%d) while fetching stats", status)
            else:
                logger.error("HTTP error %s while fetching stats: %s", status, exc)
            raise

    def get_url_stats(self, short_code: str) -> dict:
        logger.info("Fetching stats for short code: %s", short_code)
        try:
            resp = requests.get(
                f"{self.base_url}/api/stats/{short_code}", timeout=10
            )
            resp.raise_for_status()
            return resp.json()
        except requests.exceptions.Timeout:
            logger.error("Request timed out while fetching stats for: %s", short_code)
            raise
        except requests.exceptions.HTTPError as exc:
            status = exc.response.status_code if exc.response is not None else None
            if status == 404:
                logger.error("Short code not found: %s", short_code)
            elif status and status >= 500:
                logger.error("Server error (%d) while fetching URL stats", status)
            else:
                logger.error("HTTP error %s for short code %s: %s", status, short_code, exc)
            raise
