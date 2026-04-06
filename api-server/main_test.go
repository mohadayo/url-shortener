package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthCheck(t *testing.T) {
	store := NewStore("http://localhost:8080")
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
	_ = store
}

func TestShortenAndResolve(t *testing.T) {
	store := NewStore("http://localhost:8080")

	entry := store.Shorten("https://example.com")
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
	store := NewStore("http://localhost:8080")

	_, ok := store.Resolve("nonexistent")
	if ok {
		t.Error("expected not found for nonexistent code")
	}
}

func TestShortenAPI(t *testing.T) {
	store := NewStore("http://localhost:8080")
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
		entry := store.Shorten(req.URL)
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
	store := NewStore("http://localhost:8080")
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
		entry := store.Shorten(req.URL)
		resp := ShortenResponse{
			ShortURL:    "http://localhost:8080/r/" + entry.ShortCode,
			ShortCode:   entry.ShortCode,
			OriginalURL: entry.OriginalURL,
		}
		writeJSON(w, http.StatusCreated, resp)
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

func TestGetStats(t *testing.T) {
	store := NewStore("http://localhost:8080")

	store.Shorten("https://example.com")
	store.Shorten("https://example.org")

	stats := store.GetStats()
	if stats.TotalURLs != 2 {
		t.Errorf("expected 2 total URLs, got %d", stats.TotalURLs)
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
