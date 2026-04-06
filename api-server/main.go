package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"sync"
	"time"
)

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
	mu      sync.RWMutex
	urls    map[string]*URLEntry
	baseURL string
}

func NewStore(baseURL string) *Store {
	return &Store{
		urls:    make(map[string]*URLEntry),
		baseURL: baseURL,
	}
}

func (s *Store) generateCode(url string) string {
	hash := sha256.Sum256([]byte(url + time.Now().String()))
	return hex.EncodeToString(hash[:])[:8]
}

func (s *Store) Shorten(originalURL string) *URLEntry {
	s.mu.Lock()
	defer s.mu.Unlock()

	code := s.generateCode(originalURL)
	entry := &URLEntry{
		OriginalURL: originalURL,
		ShortCode:   code,
		CreatedAt:   time.Now(),
		Clicks:      0,
	}
	s.urls[code] = entry
	return entry
}

func (s *Store) Resolve(code string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	entry, ok := s.urls[code]
	if !ok {
		return "", false
	}
	entry.Clicks++
	return entry.OriginalURL, true
}

func (s *Store) GetStats() StatsResponse {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stats := StatsResponse{}
	for _, entry := range s.urls {
		stats.TotalURLs++
		stats.TotalClicks += entry.Clicks
		stats.Entries = append(stats.Entries, *entry)
	}
	return stats
}

func (s *Store) GetEntry(code string) (*URLEntry, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	entry, ok := s.urls[code]
	if !ok {
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

	store := NewStore(baseURL)
	mux := http.NewServeMux()

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

		entry := store.Shorten(req.URL)
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
