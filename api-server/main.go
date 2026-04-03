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

const maxRequestBodySize = 1 << 20 // 1MB

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
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("ERROR: failed to encode JSON response: %v", err)
	}
}

func isValidURL(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	return u.Scheme == "http" || u.Scheme == "https"
}

func logRequest(handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		log.Printf("INFO: --> %s %s", r.Method, r.URL.Path)
		handler(w, r)
		log.Printf("INFO: <-- %s %s (%v)", r.Method, r.URL.Path, time.Since(start))
	}
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
	mux.HandleFunc("/health", logRequest(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}))

	// Shorten URL
	mux.HandleFunc("/api/shorten", logRequest(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}

		r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodySize)

		var req ShortenRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			log.Printf("WARN: failed to decode shorten request: %v", err)
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}

		if req.URL == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "url is required"})
			return
		}

		if !isValidURL(req.URL) {
			log.Printf("WARN: invalid URL scheme rejected: %s", req.URL)
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "url must have http or https scheme"})
			return
		}

		entry := store.Shorten(req.URL)
		log.Printf("INFO: shortened URL %s -> %s", req.URL, entry.ShortCode)
		resp := ShortenResponse{
			ShortURL:    fmt.Sprintf("%s/r/%s", baseURL, entry.ShortCode),
			ShortCode:   entry.ShortCode,
			OriginalURL: entry.OriginalURL,
		}
		writeJSON(w, http.StatusCreated, resp)
	}))

	// Redirect
	mux.HandleFunc("/r/", logRequest(func(w http.ResponseWriter, r *http.Request) {
		code := r.URL.Path[len("/r/"):]
		if code == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "short code required"})
			return
		}

		originalURL, ok := store.Resolve(code)
		if !ok {
			log.Printf("WARN: short code not found: %s", code)
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "short url not found"})
			return
		}

		log.Printf("INFO: redirecting %s -> %s", code, originalURL)
		http.Redirect(w, r, originalURL, http.StatusTemporaryRedirect)
	}))

	// Stats
	mux.HandleFunc("/api/stats", logRequest(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}
		stats := store.GetStats()
		log.Printf("INFO: stats requested: %d URLs, %d clicks", stats.TotalURLs, stats.TotalClicks)
		writeJSON(w, http.StatusOK, stats)
	}))

	// Single URL stats
	mux.HandleFunc("/api/stats/", logRequest(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}
		code := r.URL.Path[len("/api/stats/"):]
		entry, ok := store.GetEntry(code)
		if !ok {
			log.Printf("WARN: stats for unknown short code: %s", code)
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "short url not found"})
			return
		}
		writeJSON(w, http.StatusOK, entry)
	}))

	log.Printf("URL Shortener API server starting on port %s", port)
	log.Fatal(http.ListenAndServe(":"+port, mux))
}
