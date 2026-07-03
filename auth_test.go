package webhookrelay

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestNewWithAPIKeyBearerAuth verifies that a client built with NewWithAPIKey
// authenticates using an "Authorization: Bearer <key>" header.
func TestNewWithAPIKeyBearerAuth(t *testing.T) {
	var gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("[]"))
	}))
	defer server.Close()

	client, err := NewWithAPIKey("sk-test-key", WithAPIEndpointURL(server.URL))
	if err != nil {
		t.Fatalf("NewWithAPIKey: %v", err)
	}

	if _, err := client.ListBuckets(&BucketListOptions{}); err != nil {
		t.Fatalf("ListBuckets: %v", err)
	}

	if gotAuth != "Bearer sk-test-key" {
		t.Fatalf("expected Authorization %q, got %q", "Bearer sk-test-key", gotAuth)
	}
}

// TestNewBasicAuth verifies the key/secret constructor still authenticates with
// HTTP Basic auth (and does not send a Bearer header).
func TestNewBasicAuth(t *testing.T) {
	var gotUser, gotPass string
	var gotOK bool
	var gotBearer string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUser, gotPass, gotOK = r.BasicAuth()
		gotBearer = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("[]"))
	}))
	defer server.Close()

	client, err := New("key-1", "secret-1", WithAPIEndpointURL(server.URL))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if _, err := client.ListBuckets(&BucketListOptions{}); err != nil {
		t.Fatalf("ListBuckets: %v", err)
	}

	if !gotOK || gotUser != "key-1" || gotPass != "secret-1" {
		t.Fatalf("expected basic auth key-1:secret-1, got ok=%v user=%q pass=%q", gotOK, gotUser, gotPass)
	}
	// Basic auth also lands in the Authorization header, but as "Basic ...".
	if got := gotBearer; len(got) < 6 || got[:6] != "Basic " {
		t.Fatalf("expected Basic authorization header, got %q", got)
	}
}

// TestConstructorsRejectEmptyCredentials guards the credential validation.
func TestConstructorsRejectEmptyCredentials(t *testing.T) {
	if _, err := NewWithAPIKey(""); err == nil {
		t.Fatal("expected error for empty API key")
	}
	if _, err := New("", "secret"); err == nil {
		t.Fatal("expected error for empty key")
	}
	if _, err := New("key", ""); err == nil {
		t.Fatal("expected error for empty secret")
	}
}
