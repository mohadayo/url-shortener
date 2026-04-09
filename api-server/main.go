package main

import (
	"crypto/sha256"
	"database/sql"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"time"
)

//go:embed static/index.html
var indexHTML []byte

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
	code := s.generateCode(originalURL)
	entry := &URLEntry{}
	err := s.db.QueryRow(
		`INSERT INTO urls (short_code, original_url) VALUES ($1, $2)
		 RETURNING short_code, original_url, created_at, clicks`,
		code, originalURL,
	).Scan(&entry.ShortCode, &entry.OriginalURL, &entry.CreatedAt, &entry.Clicks)
	if err != nil {
		return nil, err
	}
	return entry, nil
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

func (s *Store) GetStats() StatsResponse {
	stats := StatsResponse{Entries: []URLEntry{}}
	rows, err := s.db.Query(
		`SELECT short_code, original_url, created_at, clicks FROM urls ORDER BY created_at DESC`,
	)
	if err != nil {
		return stats
	}
	defer rows.Close()

	for rows.Next() {
		var entry URLEntry
		if err := rows.Scan(&entry.ShortCode, &entry.OriginalURL, &entry.CreatedAt, &entry.Clicks); err != nil {
			continue
		}
		stats.TotalURLs++
		stats.TotalClicks += entry.Clicks
		stats.Entries = append(stats.Entries, entry)
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

// validateURL はURLが有効かつhttp/httpsスキームを持つかを検証する
func validateURL(rawURL string) error {
	if rawURL == "" {
		return fmt.Errorf("URLは必須です")
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
	return nil
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
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	// Shorten URL
	mux.HandleFunc("/api/shorten", func(w http.ResponseWriter, r *http.Request) {
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

	// Redirect
	mux.HandleFunc("/r/", func(w http.ResponseWriter, r *http.Request) {
		code := r.URL.Path[len("/r/"):]
		if code == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "short code required"})
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
		stats := store.GetStats()
		writeJSON(w, http.StatusOK, stats)
	})

	// Single URL stats
	mux.HandleFunc("/api/stats/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}
		code := r.URL.Path[len("/api/stats/"):]
		entry, ok := store.GetEntry(code)
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "short url not found"})
			return
		}
		writeJSON(w, http.StatusOK, entry)
	})

	log.Printf("URL Shortener API server starting on port %s", port)
	log.Fatal(http.ListenAndServe(":"+port, loggingMiddleware(mux)))
}
