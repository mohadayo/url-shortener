"""Tests for the analytics module."""

import unittest
from analyzer import extract_domains, top_urls_by_clicks, clicks_over_time, generate_report


SAMPLE_ENTRIES = [
    {
        "original_url": "https://example.com/page1",
        "short_code": "abc123",
        "created_at": "2025-01-15T10:00:00Z",
        "clicks": 42,
    },
    {
        "original_url": "https://example.com/page2",
        "short_code": "def456",
        "created_at": "2025-01-15T11:00:00Z",
        "clicks": 18,
    },
    {
        "original_url": "https://github.com/repo",
        "short_code": "ghi789",
        "created_at": "2025-01-16T09:00:00Z",
        "clicks": 100,
    },
    {
        "original_url": "https://docs.python.org/3/",
        "short_code": "jkl012",
        "created_at": "2025-01-16T12:00:00Z",
        "clicks": 5,
    },
]


class TestExtractDomains(unittest.TestCase):
    def test_counts_domains(self):
        domains = extract_domains(SAMPLE_ENTRIES)
        self.assertEqual(domains["example.com"], 2)
        self.assertEqual(domains["github.com"], 1)
        self.assertEqual(domains["docs.python.org"], 1)

    def test_empty_entries(self):
        domains = extract_domains([])
        self.assertEqual(len(domains), 0)


class TestTopURLs(unittest.TestCase):
    def test_returns_sorted(self):
        top = top_urls_by_clicks(SAMPLE_ENTRIES, 2)
        self.assertEqual(len(top), 2)
        self.assertEqual(top[0]["short_code"], "ghi789")
        self.assertEqual(top[1]["short_code"], "abc123")

    def test_n_larger_than_entries(self):
        top = top_urls_by_clicks(SAMPLE_ENTRIES, 100)
        self.assertEqual(len(top), 4)


class TestClicksOverTime(unittest.TestCase):
    def test_groups_by_date(self):
        daily = clicks_over_time(SAMPLE_ENTRIES)
        self.assertEqual(daily["2025-01-15"], 60)
        self.assertEqual(daily["2025-01-16"], 105)

    def test_empty_entries(self):
        daily = clicks_over_time([])
        self.assertEqual(len(daily), 0)


class TestGenerateReport(unittest.TestCase):
    def test_report_contains_totals(self):
        stats = {"total_urls": 4, "total_clicks": 165, "entries": SAMPLE_ENTRIES}
        report = generate_report(stats)
        self.assertIn("Total URLs:   4", report)
        self.assertIn("Total Clicks: 165", report)

    def test_report_no_entries(self):
        stats = {"total_urls": 0, "total_clicks": 0, "entries": []}
        report = generate_report(stats)
        self.assertIn("Total URLs:   0", report)


if __name__ == "__main__":
    unittest.main()
