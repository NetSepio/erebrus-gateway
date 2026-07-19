package mailer

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSendOTPPostsToResend(t *testing.T) {
	var gotAuth, gotPath, gotCT string
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotPath = r.URL.Path
		gotCT = r.Header.Get("Content-Type")
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &body)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"email_123"}`))
	}))
	defer srv.Close()

	m := New("re_test_key", "Erebrus <no-reply@erebrus.network>")
	m.baseURL = srv.URL // white-box override for the test

	if err := m.SendOTP(context.Background(), "user@example.com", "123456", ""); err != nil {
		t.Fatalf("SendOTP: %v", err)
	}
	if gotPath != "/emails" {
		t.Fatalf("path = %q, want /emails", gotPath)
	}
	if gotAuth != "Bearer re_test_key" {
		t.Fatalf("auth = %q, want Bearer re_test_key", gotAuth)
	}
	if gotCT != "application/json" {
		t.Fatalf("content-type = %q", gotCT)
	}
	if to, _ := body["to"].([]any); len(to) != 1 || to[0] != "user@example.com" {
		t.Fatalf("to = %v", body["to"])
	}
	// The code must appear in the body so the user can read it.
	if txt, _ := body["text"].(string); !contains(txt, "123456") {
		t.Fatalf("text missing code: %q", txt)
	}
}

func TestSendOrgInviteUsesBrandedHTML(t *testing.T) {
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	m := New("re_test_key", "")
	m.baseURL = srv.URL
	data := OrgInviteEmail{
		OrgName:     "Acme Corp",
		InviterName: "Alex",
		Role:        "Manager",
		InviteURL:   "https://erebrus.io/notifications/invite/org-1",
	}
	if err := m.SendOrgInvite(context.Background(), "user@example.com", data); err != nil {
		t.Fatalf("SendOrgInvite: %v", err)
	}
	html, _ := body["html"].(string)
	for _, want := range []string{"Erebrus", "Acme Corp", "Alex", "Manager", "NetSepio LLC", "https://erebrus.io/favicon.ico"} {
		if !contains(html, want) {
			t.Fatalf("html missing %q: %.200s…", want, html)
		}
	}
}

func TestDisabledMailer(t *testing.T) {
	m := New("", "")
	if m.Enabled() {
		t.Fatal("empty api key should be disabled")
	}
	if err := m.SendOTP(context.Background(), "user@example.com", "123456", ""); err != ErrDisabled {
		t.Fatalf("want ErrDisabled, got %v", err)
	}
}

func TestSendOTPNon2xxIsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`{"message":"invalid from"}`))
	}))
	defer srv.Close()

	m := New("re_test_key", "")
	m.baseURL = srv.URL
	if err := m.SendOTP(context.Background(), "user@example.com", "123456", ""); err == nil {
		t.Fatal("expected error on 422")
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
