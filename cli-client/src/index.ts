#!/usr/bin/env node

import { APIClient } from "./api.js";

function printUsage(): void {
  console.log(`
URL Shortener CLI

Usage:
  urlshort shorten <url>          Shorten a URL
  urlshort stats                  Show all statistics
  urlshort lookup <code>          Look up a specific short code
  urlshort health                 Check API server health

Options:
  --api-url <url>                 API server URL (default: http://localhost:8080)
  --help                          Show this help message
`);
}

function parseArgs(args: string[]): { command: string; positional: string[]; apiURL?: string } {
  let apiURL: string | undefined;
  const positional: string[] = [];
  let command = "";

  for (let i = 0; i < args.length; i++) {
    if (args[i] === "--api-url" && i + 1 < args.length) {
      apiURL = args[++i];
    } else if (args[i] === "--help") {
      printUsage();
      process.exit(0);
    } else if (!command) {
      command = args[i];
    } else {
      positional.push(args[i]);
    }
  }

  return { command, positional, apiURL };
}

async function main(): Promise<void> {
  const args = process.argv.slice(2);

  if (args.length === 0) {
    printUsage();
    process.exit(1);
  }

  const { command, positional, apiURL } = parseArgs(args);
  const client = new APIClient(apiURL);

  switch (command) {
    case "shorten": {
      if (positional.length === 0) {
        console.error("Error: URL is required");
        process.exit(1);
      }
      const result = await client.shorten(positional[0]);
      console.log(`Short URL: ${result.short_url}`);
      console.log(`Code:      ${result.short_code}`);
      console.log(`Original:  ${result.original_url}`);
      break;
    }

    case "stats": {
      const stats = await client.getStats();
      console.log(`Total URLs:   ${stats.total_urls}`);
      console.log(`Total Clicks: ${stats.total_clicks}`);
      if (stats.entries && stats.entries.length > 0) {
        console.log("\nAll URLs:");
        console.log("-".repeat(70));
        for (const entry of stats.entries) {
          console.log(`  ${entry.short_code}  ${String(entry.clicks).padStart(5)} clicks  ${entry.original_url}`);
        }
      }
      break;
    }

    case "lookup": {
      if (positional.length === 0) {
        console.error("Error: short code is required");
        process.exit(1);
      }
      const entry = await client.getURLStats(positional[0]);
      console.log(`Code:     ${entry.short_code}`);
      console.log(`URL:      ${entry.original_url}`);
      console.log(`Clicks:   ${entry.clicks}`);
      console.log(`Created:  ${entry.created_at}`);
      break;
    }

    case "health": {
      const healthy = await client.healthCheck();
      if (healthy) {
        console.log("API server is healthy");
      } else {
        console.error("API server is not reachable");
        process.exit(1);
      }
      break;
    }

    default:
      console.error(`Unknown command: ${command}`);
      printUsage();
      process.exit(1);
  }
}

main().catch((err) => {
  console.error(`Error: ${err.message}`);
  process.exit(1);
});
