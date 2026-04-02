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

export class APIClient {
  private baseURL: string;

  constructor(baseURL?: string) {
    this.baseURL = (baseURL || DEFAULT_BASE_URL).replace(/\/+$/, "");
  }

  async shorten(url: string): Promise<ShortenResponse> {
    const resp = await fetch(`${this.baseURL}/api/shorten`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ url }),
    });
    if (!resp.ok) {
      const err = await resp.json();
      throw new Error(err.error || `HTTP ${resp.status}`);
    }
    return resp.json();
  }

  async getStats(): Promise<StatsResponse> {
    const resp = await fetch(`${this.baseURL}/api/stats`);
    if (!resp.ok) {
      const err = await resp.json();
      throw new Error(err.error || `HTTP ${resp.status}`);
    }
    return resp.json();
  }

  async getURLStats(code: string): Promise<URLEntry> {
    const resp = await fetch(`${this.baseURL}/api/stats/${code}`);
    if (!resp.ok) {
      const err = await resp.json();
      throw new Error(err.error || `HTTP ${resp.status}`);
    }
    return resp.json();
  }

  async healthCheck(): Promise<boolean> {
    try {
      const resp = await fetch(`${this.baseURL}/health`);
      return resp.ok;
    } catch {
      return false;
    }
  }
}
