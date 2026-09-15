package ratelimit

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestBurstThen429(t *testing.T) {
	l := New(1, 2)
	now := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	if !l.Allow("a", now) || !l.Allow("a", now) {
		t.Fatal("burst of 2 must pass")
	}
	if l.Allow("a", now) {
		t.Fatal("third request in the same instant must 429")
	}
	if !l.Allow("a", now.Add(time.Second)) {
		t.Fatal("one token refills each second")
	}
}

func TestHealthExempt(t *testing.T) {
	l := New(1, 1)
	h := l.Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	for i := 0; i < 5; i++ {
		req := httptest.NewRequest(http.MethodGet, "/health", nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusNoContent {
			t.Fatalf("health %d: %d", i, rec.Code)
		}
	}
	req := httptest.NewRequest(http.MethodGet, "/portfolio", nil)
	req.RemoteAddr = "10.0.0.1:9"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("first portfolio %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("want 429 got %d", rec.Code)
	}
}
