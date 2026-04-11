package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"
	"time"

	_ "github.com/lib/pq"
)

func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set, skipping database test")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	if err := db.Ping(); err != nil {
		t.Skipf("database not reachable: %v", err)
	}
	db.Exec(`DROP TABLE IF EXISTS urls`)
	_, err = db.Exec(`
		CREATE TABLE urls (
			short_code VARCHAR(8) PRIMARY KEY,
			original_url TEXT NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			clicks INTEGER NOT NULL DEFAULT 0
		)
	`)
	if err != nil {
		t.Fatalf("failed to create table: %v", err)
	}
	t.Cleanup(func() {
		db.Exec(`DROP TABLE IF EXISTS urls`)
		db.Close()
	})
	return db
}

func TestHealthCheck(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["status"] != "ok" {
		t.Errorf("expected status ok, got %s", resp["status"])
	}
}

func TestShortenAndResolve(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore("http://localhost:8080", db)

	entry, err := store.Shorten("https://example.com")
	if err != nil {
		t.Fatalf("failed to shorten: %v", err)
	}
	if entry.ShortCode == "" {
		t.Fatal("expected non-empty short code")
	}
	if entry.OriginalURL != "https://example.com" {
		t.Errorf("expected original URL https://example.com, got %s", entry.OriginalURL)
	}
	if entry.Clicks != 0 {
		t.Errorf("expected 0 clicks, got %d", entry.Clicks)
	}

	url, ok := store.Resolve(entry.ShortCode)
	if !ok {
		t.Fatal("expected to resolve short code")
	}
	if url != "https://example.com" {
		t.Errorf("expected https://example.com, got %s", url)
	}

	resolvedEntry, _ := store.GetEntry(entry.ShortCode)
	if resolvedEntry.Clicks != 1 {
		t.Errorf("expected 1 click after resolve, got %d", resolvedEntry.Clicks)
	}
}

func TestResolveNotFound(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore("http://localhost:8080", db)

	_, ok := store.Resolve("nonexistent")
	if ok {
		t.Error("expected not found for nonexistent code")
	}
}

func TestShortenAPI(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore("http://localhost:8080", db)
	mux := http.NewServeMux()

	mux.HandleFunc("/api/shorten", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}
		var req ShortenRequest
		json.NewDecoder(r.Body).Decode(&req)
		if err := validateURL(req.URL); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		entry, err := store.Shorten(req.URL)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to shorten URL"})
			return
		}
		resp := ShortenResponse{
			ShortURL:    "http://localhost:8080/r/" + entry.ShortCode,
			ShortCode:   entry.ShortCode,
			OriginalURL: entry.OriginalURL,
		}
		writeJSON(w, http.StatusCreated, resp)
	})

	body := bytes.NewBufferString(`{"url":"https://example.com"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/shorten", body)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected status 201, got %d", w.Code)
	}

	var resp ShortenResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.OriginalURL != "https://example.com" {
		t.Errorf("expected original URL https://example.com, got %s", resp.OriginalURL)
	}
	if resp.ShortCode == "" {
		t.Error("expected non-empty short code")
	}
}

func TestShortenAPIInvalidURL(t *testing.T) {
	mux := http.NewServeMux()

	mux.HandleFunc("/api/shorten", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}
		var req ShortenRequest
		json.NewDecoder(r.Body).Decode(&req)
		if err := validateURL(req.URL); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusCreated, map[string]string{})
	})

	tests := []struct {
		name    string
		payload string
	}{
		{"空のURL", `{"url":""}`},
		{"無効なURL", `{"url":"not-a-url"}`},
		{"ftpスキーム", `{"url":"ftp://example.com"}`},
		{"ホスト名なし", `{"url":"http://"}`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			body := bytes.NewBufferString(tc.payload)
			req := httptest.NewRequest(http.MethodPost, "/api/shorten", body)
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, req)

			if w.Code != http.StatusBadRequest {
				t.Errorf("expected status 400 for %s, got %d", tc.name, w.Code)
			}
		})
	}
}

func TestValidateURL(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{"有効なhttps URL", "https://example.com", false},
		{"有効なhttp URL", "http://example.com/path?q=1", false},
		{"空文字列", "", true},
		{"スキームなし", "example.com", true},
		{"ftpスキーム", "ftp://example.com", true},
		{"ホスト名なし", "http://", true},
		{"無効な形式", "://invalid", true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateURL(tc.url)
			if (err != nil) != tc.wantErr {
				t.Errorf("validateURL(%q) error = %v, wantErr %v", tc.url, err, tc.wantErr)
			}
		})
	}
}

func TestValidateURLBlocksPrivateIPs(t *testing.T) {
	tests := []struct {
		name string
		url  string
	}{
		{"ループバックIPv4", "http://127.0.0.1/secret"},
		{"ループバックlocalhost", "http://127.0.0.1:8080/admin"},
		{"プライベートIP 10.x", "http://10.0.0.1/internal"},
		{"プライベートIP 192.168.x", "http://192.168.1.100/private"},
		{"プライベートIP 172.16.x", "http://172.16.0.1/internal"},
		{"リンクローカル", "http://169.254.169.254/metadata"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateURL(tc.url)
			if err == nil {
				t.Errorf("validateURL(%q) should have returned an error for private IP", tc.url)
			}
		})
	}
}

func TestIsPrivateIP(t *testing.T) {
	tests := []struct {
		name      string
		ipStr     string
		isPrivate bool
	}{
		{"ループバック 127.0.0.1", "127.0.0.1", true},
		{"プライベート 10.0.0.1", "10.0.0.1", true},
		{"プライベート 192.168.0.1", "192.168.0.1", true},
		{"プライベート 172.16.0.1", "172.16.0.1", true},
		{"リンクローカル 169.254.1.1", "169.254.1.1", true},
		{"パブリック 8.8.8.8", "8.8.8.8", false},
		{"パブリック 1.1.1.1", "1.1.1.1", false},
		{"IPv6ループバック ::1", "::1", true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ip := net.ParseIP(tc.ipStr)
			if ip == nil {
				t.Fatalf("invalid test IP: %s", tc.ipStr)
			}
			result := isPrivateIP(ip)
			if result != tc.isPrivate {
				t.Errorf("isPrivateIP(%s) = %v, want %v", tc.ipStr, result, tc.isPrivate)
			}
		})
	}
}

func TestRateLimiter(t *testing.T) {
	// 3リクエスト/秒のレート制限でテスト
	rl := NewRateLimiter(3, time.Second)

	ip := "192.0.2.1"

	// 最初の3リクエストは許可されるべき
	for i := 0; i < 3; i++ {
		allowed, remaining, _ := rl.Allow(ip)
		if !allowed {
			t.Errorf("request %d should be allowed", i+1)
		}
		expectedRemaining := 3 - (i + 1)
		if remaining != expectedRemaining {
			t.Errorf("request %d: expected %d remaining, got %d", i+1, expectedRemaining, remaining)
		}
	}

	// 4番目のリクエストは拒否されるべき
	allowed, remaining, _ := rl.Allow(ip)
	if allowed {
		t.Error("4th request should be blocked")
	}
	if remaining != 0 {
		t.Errorf("expected 0 remaining, got %d", remaining)
	}
}

func TestRateLimiterWindowReset(t *testing.T) {
	// 1リクエスト/10ミリ秒のレート制限でテスト
	rl := NewRateLimiter(1, 10*time.Millisecond)

	ip := "192.0.2.2"

	// 最初のリクエストは許可
	allowed, _, _ := rl.Allow(ip)
	if !allowed {
		t.Error("first request should be allowed")
	}

	// 2番目のリクエストは拒否
	allowed, _, _ = rl.Allow(ip)
	if allowed {
		t.Error("second request should be blocked")
	}

	// ウィンドウが過ぎた後は再び許可
	time.Sleep(15 * time.Millisecond)
	allowed, _, _ = rl.Allow(ip)
	if !allowed {
		t.Error("request after window reset should be allowed")
	}
}

func TestRateLimitMiddleware(t *testing.T) {
	rl := NewRateLimiter(2, time.Minute)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	limited := rateLimitMiddleware(rl, handler)

	// 最初の2リクエストは通過
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.RemoteAddr = "192.0.2.3:1234"
		w := httptest.NewRecorder()
		limited.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Errorf("request %d: expected 200, got %d", i+1, w.Code)
		}
		if w.Header().Get("X-RateLimit-Limit") == "" {
			t.Error("X-RateLimit-Limit header should be set")
		}
	}

	// 3番目のリクエストは429を返す
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.RemoteAddr = "192.0.2.3:1234"
	w := httptest.NewRecorder()
	limited.ServeHTTP(w, req)
	if w.Code != http.StatusTooManyRequests {
		t.Errorf("expected 429, got %d", w.Code)
	}
}

func TestGetClientIP(t *testing.T) {
	tests := []struct {
		name       string
		remoteAddr string
		headers    map[string]string
		expectedIP string
	}{
		{
			name:       "X-Forwarded-Forヘッダーから取得",
			remoteAddr: "10.0.0.1:1234",
			headers:    map[string]string{"X-Forwarded-For": "203.0.113.5, 10.0.0.1"},
			expectedIP: "203.0.113.5",
		},
		{
			name:       "X-Real-IPヘッダーから取得",
			remoteAddr: "10.0.0.1:1234",
			headers:    map[string]string{"X-Real-IP": "203.0.113.10"},
			expectedIP: "203.0.113.10",
		},
		{
			name:       "RemoteAddrから取得",
			remoteAddr: "203.0.113.20:5678",
			headers:    map[string]string{},
			expectedIP: "203.0.113.20",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			req.RemoteAddr = tc.remoteAddr
			for k, v := range tc.headers {
				req.Header.Set(k, v)
			}
			ip := getClientIP(req)
			if ip != tc.expectedIP {
				t.Errorf("getClientIP() = %s, want %s", ip, tc.expectedIP)
			}
		})
	}
}

func TestGetStats(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore("http://localhost:8080", db)

	store.Shorten("https://example.com")
	store.Shorten("https://example.org")

	stats := store.GetStats(20, 0)
	if stats.TotalURLs != 2 {
		t.Errorf("expected 2 total URLs, got %d", stats.TotalURLs)
	}
}

func TestGetStatsPagination(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore("http://localhost:8080", db)

	// 5件のURLを登録
	for i := 0; i < 5; i++ {
		_, err := store.Shorten("https://example.com/" + strconv.Itoa(i))
		if err != nil {
			t.Fatalf("failed to shorten URL %d: %v", i, err)
		}
	}

	// limit=2で最初のページを取得
	stats := store.GetStats(2, 0)
	if stats.TotalURLs != 5 {
		t.Errorf("expected TotalURLs=5, got %d", stats.TotalURLs)
	}
	if len(stats.Entries) != 2 {
		t.Errorf("expected 2 entries, got %d", len(stats.Entries))
	}

	// offset=2で次のページを取得
	stats2 := store.GetStats(2, 2)
	if stats2.TotalURLs != 5 {
		t.Errorf("expected TotalURLs=5, got %d", stats2.TotalURLs)
	}
	if len(stats2.Entries) != 2 {
		t.Errorf("expected 2 entries, got %d", len(stats2.Entries))
	}

	// 最初のページと2ページ目のエントリが異なることを確認
	if len(stats.Entries) > 0 && len(stats2.Entries) > 0 {
		if stats.Entries[0].ShortCode == stats2.Entries[0].ShortCode {
			t.Error("expected different entries on different pages")
		}
	}

	// offset=4で最後のページを取得（残り1件）
	stats3 := store.GetStats(2, 4)
	if len(stats3.Entries) != 1 {
		t.Errorf("expected 1 entry on last page, got %d", len(stats3.Entries))
	}

	// offset=10で範囲外を取得（0件）
	stats4 := store.GetStats(2, 10)
	if len(stats4.Entries) != 0 {
		t.Errorf("expected 0 entries for out-of-range offset, got %d", len(stats4.Entries))
	}
	if stats4.TotalURLs != 5 {
		t.Errorf("expected TotalURLs=5 even with out-of-range offset, got %d", stats4.TotalURLs)
	}
}

func TestGetStatsTotalClicksWithPagination(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore("http://localhost:8080", db)

	// 3件のURLを登録
	entries := make([]*URLEntry, 3)
	for i := 0; i < 3; i++ {
		entry, err := store.Shorten("https://example.com/clicks/" + strconv.Itoa(i))
		if err != nil {
			t.Fatalf("failed to shorten URL %d: %v", i, err)
		}
		entries[i] = entry
	}

	// 各URLをクリック
	store.Resolve(entries[0].ShortCode) // 1クリック
	store.Resolve(entries[1].ShortCode) // 1クリック
	store.Resolve(entries[1].ShortCode) // 2クリック目

	// ページネーションで1件だけ取得しても、TotalClicksは全体の合計
	stats := store.GetStats(1, 0)
	if stats.TotalClicks != 3 {
		t.Errorf("expected TotalClicks=3, got %d", stats.TotalClicks)
	}
	if stats.TotalURLs != 3 {
		t.Errorf("expected TotalURLs=3, got %d", stats.TotalURLs)
	}
	if len(stats.Entries) != 1 {
		t.Errorf("expected 1 entry, got %d", len(stats.Entries))
	}
}

func TestStatsAPIPagination(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore("http://localhost:8080", db)

	// 3件のURLを登録
	for i := 0; i < 3; i++ {
		store.Shorten("https://example.com/api/" + strconv.Itoa(i))
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/stats", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}
		limit := 20
		offset := 0
		if v := r.URL.Query().Get("limit"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 100 {
				limit = n
			}
		}
		if v := r.URL.Query().Get("offset"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n >= 0 {
				offset = n
			}
		}
		stats := store.GetStats(limit, offset)
		writeJSON(w, http.StatusOK, stats)
	})

	// デフォルトのlimit/offsetで取得
	req := httptest.NewRequest(http.MethodGet, "/api/stats", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
	var stats StatsResponse
	json.NewDecoder(w.Body).Decode(&stats)
	if stats.TotalURLs != 3 {
		t.Errorf("expected TotalURLs=3, got %d", stats.TotalURLs)
	}
	if len(stats.Entries) != 3 {
		t.Errorf("expected 3 entries, got %d", len(stats.Entries))
	}

	// limit=1で取得
	req = httptest.NewRequest(http.MethodGet, "/api/stats?limit=1&offset=0", nil)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	var stats2 StatsResponse
	json.NewDecoder(w.Body).Decode(&stats2)
	if len(stats2.Entries) != 1 {
		t.Errorf("expected 1 entry with limit=1, got %d", len(stats2.Entries))
	}
	if stats2.TotalURLs != 3 {
		t.Errorf("expected TotalURLs=3, got %d", stats2.TotalURLs)
	}

	// 無効なlimitパラメータ（デフォルトの20が使用される）
	req = httptest.NewRequest(http.MethodGet, "/api/stats?limit=invalid", nil)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	var stats3 StatsResponse
	json.NewDecoder(w.Body).Decode(&stats3)
	if len(stats3.Entries) != 3 {
		t.Errorf("expected 3 entries with invalid limit, got %d", len(stats3.Entries))
	}

	// limitが上限を超える場合（デフォルトの20が使用される）
	req = httptest.NewRequest(http.MethodGet, "/api/stats?limit=200", nil)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	var stats4 StatsResponse
	json.NewDecoder(w.Body).Decode(&stats4)
	if len(stats4.Entries) != 3 {
		t.Errorf("expected 3 entries with limit=200 (falls back to default), got %d", len(stats4.Entries))
	}
}

func TestDBConnectionPoolSettings(t *testing.T) {
	db := setupTestDB(t)

	// 接続プール設定を適用
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	// 設定が適用されていることを確認
	if db.Stats().MaxOpenConnections != 25 {
		t.Errorf("expected MaxOpenConnections=25, got %d", db.Stats().MaxOpenConnections)
	}

	// 接続が正常に動作することを確認
	var result int
	err := db.QueryRow("SELECT 1").Scan(&result)
	if err != nil {
		t.Fatalf("query failed after pool settings: %v", err)
	}
	if result != 1 {
		t.Errorf("expected 1, got %d", result)
	}
}

func TestLoggingMiddleware(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	logged := loggingMiddleware(handler)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	logged.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}
