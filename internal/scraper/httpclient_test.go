package scraper

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestClientGet_RetriesOn503ThenSucceeds(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&hits, 1) == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte("OK"))
	}))
	defer srv.Close()

	c := NewClient(ClientOptions{Timeout: 2 * time.Second, MaxRetries: 2, MinInterval: time.Millisecond})
	body, err := c.Get(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if string(body) != "OK" || atomic.LoadInt32(&hits) != 2 {
		t.Fatalf("got body=%q hits=%d", body, hits)
	}
}

func TestClientGet_SendsUserAgentAndCookies(t *testing.T) {
	var gotUA, gotCookie string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		if ck, err := r.Cookie("cf_clearance"); err == nil {
			gotCookie = ck.Value
		}
		_, _ = w.Write([]byte("OK"))
	}))
	defer srv.Close()

	c := NewClient(ClientOptions{Timeout: time.Second, UserAgent: "Vaultflix/1.0", Cookies: map[string]string{"cf_clearance": "abc"}})
	if _, err := c.Get(context.Background(), srv.URL); err != nil {
		t.Fatal(err)
	}
	if gotUA != "Vaultflix/1.0" || gotCookie != "abc" {
		t.Fatalf("ua=%q cookie=%q", gotUA, gotCookie)
	}
}
