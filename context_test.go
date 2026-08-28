package webhookrelay

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"golang.org/x/time/rate"
)

// TestWithContextCancelsInFlightWork verifies that a cancelled context stops
// a request instead of letting it run to the client timeout — the reason
// WithContext exists.
func TestWithContextCancelsInFlightWork(t *testing.T) {
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-release // hold the request open until the test ends
	}))
	defer server.Close()
	defer close(release)

	client, err := NewWithAPIKey("sk-test", WithAPIEndpointURL(server.URL))
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err = client.WithContext(ctx).ListBuckets(&BucketListOptions{})
	if err == nil {
		t.Fatal("a cancelled context did not stop the request")
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("cancellation took %s; the context was ignored", elapsed)
	}
}

// TestWithContextDoesNotAffectTheOriginalClient: WithContext returns a copy;
// the original keeps working with a background context.
func TestWithContextDoesNotAffectTheOriginalClient(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`[]`))
	}))
	defer server.Close()

	client, err := NewWithAPIKey("sk-test", WithAPIEndpointURL(server.URL))
	if err != nil {
		t.Fatal(err)
	}

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := client.WithContext(cancelled).ListBuckets(&BucketListOptions{}); err == nil {
		t.Fatal("the bound copy ignored its cancelled context")
	}
	if _, err := client.ListBuckets(&BucketListOptions{}); err != nil {
		t.Fatalf("the original client was poisoned by the copy: %v", err)
	}
}

// TestWithRateLimitOverridesTheDefault: the default 4 rps / burst 1 is right
// for the API's default tier, and wrong for everyone above it.
func TestWithRateLimitOverridesTheDefault(t *testing.T) {
	client, err := NewWithAPIKey("sk-test", WithRateLimit(100, 10))
	if err != nil {
		t.Fatal(err)
	}
	if got := client.rateLimiter.Limit(); got != rate.Limit(100) {
		t.Fatalf("rate not applied: %v", got)
	}
	if got := client.rateLimiter.Burst(); got != 10 {
		t.Fatalf("burst not applied: %d", got)
	}

	if _, err := NewWithAPIKey("sk-test", WithRateLimit(0, 0)); err == nil {
		t.Fatal("a zero rate limit was accepted; it would deadlock every request")
	}
}
