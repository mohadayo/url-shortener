"""CLI entry point for the analytics service."""

import argparse
import sys

from client import APIClient, APIError
from analyzer import generate_report, top_urls_by_clicks, extract_domains


def main():
    parser = argparse.ArgumentParser(description="URL Shortener Analytics")
    parser.add_argument(
        "--api-url",
        default="http://localhost:8080",
        help="Base URL of the API server",
    )
    subparsers = parser.add_subparsers(dest="command")

    subparsers.add_parser("report", help="Generate full analytics report")
    subparsers.add_parser("top", help="Show top URLs by clicks")
    subparsers.add_parser("domains", help="Show top domains")

    lookup = subparsers.add_parser("lookup", help="Look up a specific short code")
    lookup.add_argument("code", help="Short code to look up")

    args = parser.parse_args()

    client = APIClient(args.api_url)

    try:
        if args.command == "report":
            stats = client.get_stats()
            print(generate_report(stats))

        elif args.command == "top":
            stats = client.get_stats()
            entries = stats.get("entries") or []
            for entry in top_urls_by_clicks(entries):
                print(f"  {entry['short_code']}  {entry['clicks']:>5} clicks  {entry['original_url']}")

        elif args.command == "domains":
            stats = client.get_stats()
            entries = stats.get("entries") or []
            for domain, count in extract_domains(entries).most_common(10):
                print(f"  {domain:<30} {count:>5} URLs")

        elif args.command == "lookup":
            entry = client.get_url_stats(args.code)
            print(f"  Code:     {entry['short_code']}")
            print(f"  URL:      {entry['original_url']}")
            print(f"  Clicks:   {entry['clicks']}")
            print(f"  Created:  {entry['created_at']}")

        else:
            parser.print_help()
            sys.exit(1)

    except APIError as e:
        print(f"エラー: {e}", file=sys.stderr)
        sys.exit(1)


if __name__ == "__main__":
    main()
