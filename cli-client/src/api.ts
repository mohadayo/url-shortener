const DEFAULT_BASE_URL = "http://localhost:8080";

export interface ShortenResponse {
  short_url: string;
  short_code: string;
  original_url: string;
}

export interface URLEntry {
  original_url: string;
  short_code: string;
  created_at: string;
  clicks: number;
}

export interface StatsResponse {
  total_urls: number;
  total_clicks: number;
  entries: URLEntry[];
}

function log(level: string, message: string): void {
  const timestamp = new Date().toISOString();
  console.error(`${timestamp} [${level}] ${message}`);
}

async function parseErrorResponse(resp: Response): Promise<string> {
  try {
    const body = await resp.json();
    return body.error || `HTTP ${resp.status}`;
  } catch {
    const text = await resp.text().catch(() => "");
    return text || `HTTP ${resp.status}`;
  }
}

export class APIClient {
  private baseURL: string;

  constructor(baseURL?: string) {
    this.baseURL = (baseURL || DEFAULT_BASE_URL).replace(/\/+$/, "");
  }

  async shorten(url: string): Promise<ShortenResponse> {
    log("INFO", `Shortening URL: ${url}`);
    const resp = await fetch(`${this.baseURL}/api/shorten`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ url }),
    });
    if (!resp.ok) {
      const errMsg = await parseErrorResponse(resp);
      log("ERROR", `Failed to shorten URL: ${errMsg}`);
      throw new Error(errMsg);
    }
    return resp.json();
  }

  async getStats(): Promise<StatsResponse> {
    log("INFO", "Fetching global stats");
    const resp = await fetch(`${this.baseURL}/api/stats`);
    if (!resp.ok) {
      const errMsg = await parseErrorResponse(resp);
      log("ERROR", `Failed to fetch stats: ${errMsg}`);
      throw new Error(errMsg);
    }
    return resp.json();
  }

  async getURLStats(code: string): Promise<URLEntry> {
    log("INFO", `Fetching stats for: ${code}`);
    const resp = await fetch(`${this.baseURL}/api/stats/${code}`);
    if (!resp.ok) {
      const errMsg = await parseErrorResponse(resp);
      log("ERROR", `Failed to fetch URL stats for ${code}: ${errMsg}`);
      throw new Error(errMsg);
    }
    return resp.json();
  }

  async healthCheck(): Promise<boolean> {
    try {
      const resp = await fetch(`${this.baseURL}/health`);
      return resp.ok;
    } catch {
      log("WARN", "Health check failed");
      return false;
    }
  }
}
