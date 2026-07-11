package dropclient

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestUploadEnforcesByteLimit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		_, _ = w.Write([]byte(`{"cid":"bafy","size_bytes":3}`))
	}))
	defer srv.Close()

	c := New()
	// 100-byte body, 10-byte cap → rejected before completing.
	body := strings.NewReader(strings.Repeat("x", 100))
	_, err := c.PutUploadContent(context.Background(), srv.URL, "t", "k", "up1", "", body, 100, 10)
	if err != ErrByteLimitExceeded {
		t.Fatalf("expected ErrByteLimitExceeded, got %v", err)
	}
}

func TestUploadWithinLimitSucceeds(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-Erebrus-Declared-Size"); got != "5" {
			t.Errorf("declared size header = %q", got)
		}
		if got := r.Header.Get("X-Erebrus-SHA256"); got != strings.Repeat("a", 64) {
			t.Errorf("sha256 header = %q", got)
		}
		n, _ := io.Copy(io.Discard, r.Body)
		if n != 5 {
			t.Errorf("server read %d bytes, want 5", n)
		}
		_, _ = w.Write([]byte(`{"cid":"bafy","size_bytes":5}`))
	}))
	defer srv.Close()

	c := New()
	res, err := c.PutUploadContent(context.Background(), srv.URL, "t", "k", "up1", strings.Repeat("a", 64), strings.NewReader("hello"), 5, 1<<20)
	if err != nil {
		t.Fatalf("upload: %v", err)
	}
	if res.CID != "bafy" || res.SizeBytes != 5 {
		t.Fatalf("result = %+v", res)
	}
}

func TestGetObjectAcceptsExactLimit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("hello"))
	}))
	defer srv.Close()

	rc, _, err := New().GetObject(context.Background(), srv.URL, "t", "k", "bafy", 5)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer rc.Close()
	data, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("read exact limit: %v", err)
	}
	if string(data) != "hello" {
		t.Fatalf("data = %q", data)
	}
}

func TestGetObjectRejectsOverflow(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("hello!"))
	}))
	defer srv.Close()

	rc, _, err := New().GetObject(context.Background(), srv.URL, "t", "k", "bafy", 5)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer rc.Close()
	data, err := io.ReadAll(rc)
	if !errors.Is(err, ErrByteLimitExceeded) {
		t.Fatalf("expected byte limit error, got %v", err)
	}
	if string(data) != "hello" {
		t.Fatalf("forwarded data = %q", data)
	}
}

func TestGetObjectStreamsWithCap(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write([]byte(strings.Repeat("y", 50)))
	}))
	defer srv.Close()

	c := New()
	rc, ct, err := c.GetObject(context.Background(), srv.URL, "t", "k", "bafy", 1<<20)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer rc.Close()
	if ct != "application/octet-stream" {
		t.Errorf("content-type = %q", ct)
	}
	data, _ := io.ReadAll(rc)
	if len(data) != 50 {
		t.Fatalf("read %d bytes, want 50", len(data))
	}
}
