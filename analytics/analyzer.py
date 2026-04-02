"""Analytics module for URL shortener statistics."""

from collections import Counter
from datetime import datetime
from urllib.parse import urlparse


def extract_domains(entries: list[dict]) -> Counter:
    """Extract and count domains from shortened URLs."""
    domains = Counter()
    for entry in entries:
        parsed = urlparse(entry["original_url"])
        domains[parsed.netloc] += 1
    return domains


def top_urls_by_clicks(entries: list[dict], n: int = 10) -> list[dict]:
    """Return top N URLs sorted by click count."""
    sorted_entries = sorted(entries, key=lambda x: x["clicks"], reverse=True)
    return sorted_entries[:n]


def clicks_over_time(entries: list[dict]) -> dict[str, int]:
    """Group URLs by creation date and sum clicks."""
    daily: dict[str, int] = {}
    for entry in entries:
        date_str = entry["created_at"][:10]
        daily[date_str] = daily.get(date_str, 0) + entry["clicks"]
    return dict(sorted(daily.items()))


def generate_report(stats: dict) -> str:
    """Generate a text report from stats data."""
    entries = stats.get("entries") or []
    total_urls = stats.get("total_urls", 0)
    total_clicks = stats.get("total_clicks", 0)

    lines = [
        "=" * 50,
        "  URL Shortener Analytics Report",
        f"  Generated: {datetime.now().strftime('%Y-%m-%d %H:%M:%S')}",
        "=" * 50,
        "",
        f"  Total URLs:   {total_urls}",
        f"  Total Clicks: {total_clicks}",
        f"  Avg Clicks:   {total_clicks / total_urls:.1f}" if total_urls > 0 else "  Avg Clicks:   N/A",
        "",
    ]

    if entries:
        lines.append("  Top URLs by Clicks:")
        lines.append("  " + "-" * 46)
        for entry in top_urls_by_clicks(entries, 5):
            lines.append(
                f"    {entry['short_code']}  {entry['clicks']:>5} clicks  {entry['original_url'][:40]}"
            )
        lines.append("")

        domains = extract_domains(entries)
        lines.append("  Top Domains:")
        lines.append("  " + "-" * 46)
        for domain, count in domains.most_common(5):
            lines.append(f"    {domain:<30} {count:>5} URLs")
        lines.append("")

        daily = clicks_over_time(entries)
        if daily:
            lines.append("  Daily Click Summary:")
            lines.append("  " + "-" * 46)
            for date, clicks in list(daily.items())[-7:]:
                bar = "#" * min(clicks, 40)
                lines.append(f"    {date}  {clicks:>5}  {bar}")

    lines.append("")
    lines.append("=" * 50)
    return "\n".join(lines)
