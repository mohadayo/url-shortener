package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"regexp"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

//go:embed static/index.html
var indexHTML []byte

const maxURLLength = 2048

var validShortCode = regexp.MustCompile(`^[a-f0-9]{8}$`)

type URLEntry struct {
	OriginalURL string    `json:"original_url"`
	ShortCode   string    `json:"short_code"`
	CreatedAt   time.Time `json:"created_at"`
	Clicks      int       `json:"clicks"`
}

type ShortenRequest struct {
	URL string `json:"url"`
}

type ShortenResponse struct {
	ShortURL    string `json:"short_url"`
	ShortCode   string `json:"short_code"`
	OriginalURL string `json:"original_url"`
}

type StatsResponse struct {
	TotalURLs   int        `json:"total_urls"`
	TotalClicks int        `json:"total_clicks"`
	Entries     []URLEntry `json:"entries"`
}

type Store struct {
	db      *sql.DB
	baseURL string
}

func NewStore(baseURL string, db *sql.DB) *Store {
	return &Store{
		db:      db,
		baseURL: baseURL,
	}
}

func (s *Store) generateCode(rawURL string) string {
	hash := sha256.Sum256([]byte(rawURL + time.Now().String()))
	return hex.EncodeToString(hash[:])[:8]
}

func (s *Store) Shorten(originalURL string) (*URLEntry, error) {
	const maxRetries = 5
	for i := 0; i < maxRetries; i++ {
		code := s.generateCode(originalURL)
		entry := &URLEntry{}
		err := s.db.QueryRow(
			`INSERT INTO urls (short_code, original_url) VALUES ($1, $2)
			 RETURNING short_code, original_url, created_at, clicks`,
			code, originalURL,
		).Scan(&entry.ShortCode, &entry.OriginalURL, &entry.CreatedAt, &entry.Clicks)
		if err != nil {
			// ユニーク制約違反（ショートコード衝突）の場合はリトライ
			if strings.Contains(err.Error(), "duplicate key") ||
				strings.Contains(err.Error(), "unique") {
				log.Printf("ショートコード衝突 (試行 %d/%d): code=%s", i+1, maxRetries, code)
				continue
			}
			return nil, err
		}
		if i > 0 {
			log.Printf("ショートコード衝突回避成功: %d回目で生成 code=%s", i+1, code)
		}
		return entry, nil
	}
	return nil, fmt.Errorf("ショートコード生成に%d回失敗しました（衝突が多すぎます）", maxRetries)
}

func (s *Store) Resolve(code string) (string, bool) {
	var originalURL string
	err := s.db.QueryRow(
		`UPDATE urls SET clicks = clicks + 1 WHERE short_code = $1 RETURNING original_url`,
		code,
	).Scan(&originalURL)
	if err != nil {
		return "", false
	}
	return originalURL, true
}

func (s *Store) GetStats(limit, offset int) StatsResponse {
	stats := StatsResponse{Entries: []URLEntry{}}

	if err := s.db.QueryRow(
		`SELECT COUNT(*), COALESCE(SUM(clicks), 0) FROM urls`,
	).Scan(&stats.TotalURLs, &stats.TotalClicks); err != nil {
		log.Printf("統計集計クエリ失敗: %v", err)
		return stats
	}

	rows, err := s.db.Query(
		`SELECT short_code, original_url, created_at, clicks FROM urls ORDER BY created_at DESC LIMIT $1 OFFSET $2`,
		limit, offset,
	)
	if err != nil {
		log.Printf("統計一覧クエリ失敗: %v", err)
		return stats
	}
	defer rows.Close()

	for rows.Next() {
		var entry URLEntry
		if err := rows.Scan(&entry.ShortCode, &entry.OriginalURL, &entry.CreatedAt, &entry.Clicks); err != nil {
			log.Printf("統計行スキャン失敗: %v", err)
			continue
		}
		stats.Entries = append(stats.Entries, entry)
	}
	if err := rows.Err(); err != nil {
		log.Printf("統計行イテレーションエラー: %v", err)
	}
	return stats
}

func (s *Store) GetEntry(code string) (*URLEntry, bool) {
	entry := &URLEntry{}
	err := s.db.QueryRow(
		`SELECT short_code, original_url, created_at, clicks FROM urls WHERE short_code = $1`,
		code,
	).Scan(&entry.ShortCode, &entry.OriginalURL, &entry.CreatedAt, &entry.Clicks)
	if err != nil {
		return nil, false
	}
	return entry, true
}

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

// privateIPRanges はブロック対象のプライベートIPレンジ（SSRF対策）
var privateIPRanges []*net.IPNet

func init() {
	privateRanges := []string{
		"127.0.0.0/8",    // ループバック (IPv4)
		"::1/128",        // ループバック (IPv6)
		"10.0.0.0/8",     // プライベート (RFC1918)
		"172.16.0.0/12",  // プライベート (RFC1918)
		"192.168.0.0/16", // プライベート (RFC1918)
		"169.254.0.0/16", // リンクローカル (IPv4)
		"fe80::/10",      // リンクローカル (IPv6)
		"fc00::/7",       // ユニークローカル (IPv6)
		"0.0.0.0/8",      // "このネットワーク"
		"100.64.0.0/10",  // 共有アドレス空間 (RFC6598)
	}
	for _, cidr := range privateRanges {
		_, ipNet, err := net.ParseCIDR(cidr)
		if err == nil {
			privateIPRanges = append(privateIPRanges, ipNet)
		}
	}
}

// isPrivateIP は指定されたIPアドレスがプライベート/ローカルアドレスかどうかを返す
func isPrivateIP(ip net.IP) bool {
	for _, ipNet := range privateIPRanges {
		if ipNet.Contains(ip) {
			return true
		}
	}
	return false
}

// validateURL はURLが有効かつhttp/httpsスキームを持つかを検証し、SSRF攻撃を防ぐ
func validateURL(rawURL string) error {
	if rawURL == "" {
		return fmt.Errorf("URLは必須です")
	}
	if len(rawURL) > maxURLLength {
		return fmt.Errorf("URLは%d文字以内である必要があります", maxURLLength)
	}
	parsed, err := url.ParseRequestURI(rawURL)
	if err != nil {
		return fmt.Errorf("URLの形式が無効です: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("URLはhttpまたはhttpsスキームである必要があります")
	}
	if parsed.Host == "" {
		return fmt.Errorf("URLにホスト名が必要です")
	}

	// ホスト名からポートを除去してIPアドレスを取得
	hostname := parsed.Hostname()

	// IPアドレスとして直接解析
	ip := net.ParseIP(hostname)
	if ip != nil {
		if isPrivateIP(ip) {
			return fmt.Errorf("プライベートIPアドレスやローカルアドレスへのURLは許可されていません")
		}
		return nil
	}

	// ホスト名をDNS解決してIPアドレスを確認（3秒タイムアウト付き）
	// DNS解決に失敗した場合は許可（ネットワーク環境の制約を考慮）
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	resolver := &net.Resolver{}
	ips, err := resolver.LookupHost(ctx, hostname)
	if err == nil {
		for _, resolvedIP := range ips {
			ip := net.ParseIP(resolvedIP)
			if ip != nil && isPrivateIP(ip) {
				return fmt.Errorf("プライベートIPアドレスやローカルアドレスへのURLは許可されていません")
			}
		}
	}

	return nil
}

// RateLimiter はIPアドレスベースのレート制限を管理する
type RateLimiter struct {
	mu       sync.Mutex
	clients  map[string]*clientState
	limit    int
	window   time.Duration
}

type clientState struct {
	count    int
	windowStart time.Time
}

// NewRateLimiter は新しいレート制限器を作成する
// limit: ウィンドウ内の最大リクエスト数
// window: ウィンドウの長さ
func NewRateLimiter(limit int, window time.Duration) *RateLimiter {
	rl := &RateLimiter{
		clients: make(map[string]*clientState),
		limit:   limit,
		window:  window,
	}
	// 古いエントリを定期的にクリーンアップ
	go rl.cleanup()
	return rl
}

func (rl *RateLimiter) cleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	for range ticker.C {
		rl.mu.Lock()
		now := time.Now()
		for ip, state := range rl.clients {
			if now.Sub(state.windowStart) > rl.window*2 {
				delete(rl.clients, ip)
			}
		}
		rl.mu.Unlock()
	}
}

// Allow はリクエストが許可されるかどうかを確認し、残りリクエスト数とリセット時刻を返す
func (rl *RateLimiter) Allow(ip string) (allowed bool, remaining int, resetAt time.Time) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	state, exists := rl.clients[ip]

	if !exists || now.Sub(state.windowStart) > rl.window {
		// 新しいウィンドウを開始
		rl.clients[ip] = &clientState{
			count:       1,
			windowStart: now,
		}
		return true, rl.limit - 1, now.Add(rl.window)
	}

	state.count++
	resetAt = state.windowStart.Add(rl.window)

	if state.count > rl.limit {
		return false, 0, resetAt
	}

	return true, rl.limit - state.count, resetAt
}

// getClientIP はリクエストからクライアントIPを取得する
func getClientIP(r *http.Request) string {
	// X-Forwarded-For ヘッダーを確認（プロキシ経由の場合）
	forwarded := r.Header.Get("X-Forwarded-For")
	if forwarded != "" {
		// 最初のIPを使用（クライアントのオリジナルIP）
		parts := strings.Split(forwarded, ",")
		if len(parts) > 0 {
			ip := strings.TrimSpace(parts[0])
			if net.ParseIP(ip) != nil {
				return ip
			}
		}
	}

	// X-Real-IP ヘッダーを確認
	realIP := r.Header.Get("X-Real-IP")
	if realIP != "" && net.ParseIP(realIP) != nil {
		return realIP
	}

	// RemoteAddrから取得
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}

// rateLimitMiddleware はレート制限を適用するミドルウェア
func rateLimitMiddleware(rl *RateLimiter, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := getClientIP(r)
		allowed, remaining, resetAt := rl.Allow(ip)

		w.Header().Set("X-RateLimit-Limit", fmt.Sprintf("%d", rl.limit))
		w.Header().Set("X-RateLimit-Remaining", fmt.Sprintf("%d", remaining))
		w.Header().Set("X-RateLimit-Reset", fmt.Sprintf("%d", resetAt.Unix()))

		if !allowed {
			writeJSON(w, http.StatusTooManyRequests, map[string]string{
				"error": "リクエスト数の上限に達しました。しばらく待ってから再試行してください。",
			})
			return
		}

		next.ServeHTTP(w, r)
	})
}

// loggingMiddleware はHTTPリクエストをログ出力するミドルウェア
func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		wrapped := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}
		next.ServeHTTP(wrapped, r)
		duration := time.Since(start)
		log.Printf("%s %s %d %s %s", r.Method, r.URL.Path, wrapped.statusCode, duration, r.RemoteAddr)
	})
}

// responseWriter はステータスコードをキャプチャするためのラッパー
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	baseURL := os.Getenv("BASE_URL")
	if baseURL == "" {
		baseURL = fmt.Sprintf("http://localhost:%s", port)
	}

	db, err := initDB()
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer db.Close()

	store := NewStore(baseURL, db)
	// レート制限: 1分間に60リクエストまで
	rateLimiter := NewRateLimiter(60, time.Minute)
	mux := http.NewServeMux()

	// Root - serve frontend
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(indexHTML)
	})

	// Health check
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		dbStatus := "ok"
		if err := db.PingContext(ctx); err != nil {
			dbStatus = "error"
		}
		status := "ok"
		if dbStatus != "ok" {
			status = "degraded"
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": status, "db": dbStatus})
	})

	// Shorten URL（レート制限適用）
	shortenHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}

		var req ShortenRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}

		if err := validateURL(req.URL); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}

		entry, err := store.Shorten(req.URL)
		if err != nil {
			log.Printf("Failed to shorten URL: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to shorten URL"})
			return
		}

		resp := ShortenResponse{
			ShortURL:    fmt.Sprintf("%s/r/%s", baseURL, entry.ShortCode),
			ShortCode:   entry.ShortCode,
			OriginalURL: entry.OriginalURL,
		}
		writeJSON(w, http.StatusCreated, resp)
	})
	mux.Handle("/api/shorten", rateLimitMiddleware(rateLimiter, shortenHandler))

	// Redirect
	mux.HandleFunc("/r/", func(w http.ResponseWriter, r *http.Request) {
		code := r.URL.Path[len("/r/"):]
		if code == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "short code required"})
			return
		}
		if !validShortCode.MatchString(code) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid short code format"})
			return
		}

		originalURL, ok := store.Resolve(code)
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "short url not found"})
			return
		}

		http.Redirect(w, r, originalURL, http.StatusTemporaryRedirect)
	})

	// Stats
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

	// Single URL stats
	mux.HandleFunc("/api/stats/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}
		code := r.URL.Path[len("/api/stats/"):]
		if !validShortCode.MatchString(code) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid short code format"})
			return
		}
		entry, ok := store.GetEntry(code)
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "short url not found"})
			return
		}
		writeJSON(w, http.StatusOK, entry)
	})

	srv := &http.Server{
		Addr:         ":" + port,
		Handler:      loggingMiddleware(mux),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		log.Printf("URL Shortener API server starting on port %s", port)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

	<-quit
	log.Println("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}
	log.Println("Server stopped gracefully")
}
