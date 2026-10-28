package webhookrelay

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// retryTestClient points a client at handler, which is given the request's
// 1-based number, with a fast retry policy: 3 retries, 10ms minimum and 2s
// maximum delay. It returns the client and the number of requests the
// server received.
func retryTestClient(t *testing.T, handler func(w http.ResponseWriter, attempt int32)) (*API, *atomic.Int32) {
	t.Helper()
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handler(w, requests.Add(1))
	}))
	t.Cleanup(server.Close)

	client, err := NewWithAPIKey("sk-test", WithAPIEndpointURL(server.URL), WithRateLimit(1000, 10))
	if err != nil {
		t.Fatal(err)
	}
	client.retryPolicy = RetryPolicy{MaxRetries: 3, MinRetryDelay: 10 * time.Millisecond, MaxRetryDelay: 2 * time.Second}
	return client, &requests
}

func requireAPIStatus(t *testing.T, err error, status int) {
	t.Helper()
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected an *APIError, got %v", err)
	}
	if apiErr.StatusCode != status {
		t.Fatalf("expected status %d, got %d", status, apiErr.StatusCode)
	}
}

func TestQuotaExhausted429IsNotRetried(t *testing.T) {
	for name, body := range map[string]string{
		"outbound messages": `{"error":"monthly outbound message quota reached: 1000 messages this billing month, renewing 2026-11-01; add an outbound message pack or upgrade at https://my.webhookrelay.com/plans"}`,
		"agent tokens":      `{"error":"this month's agent token allowance is used up","code":"agent_quota_exceeded","quota":{}}`,
	} {
		t.Run(name, func(t *testing.T) {
			client, requests := retryTestClient(t, func(w http.ResponseWriter, _ int32) {
				w.WriteHeader(http.StatusTooManyRequests)
				w.Write([]byte(body))
			})

			_, err := client.ListBuckets(&BucketListOptions{})
			requireAPIStatus(t, err, http.StatusTooManyRequests)
			if got := requests.Load(); got != 1 {
				t.Fatalf("expected 1 request, got %d", got)
			}
		})
	}
}

func TestRetryAfterWithinMaxDelayIsHonoured(t *testing.T) {
	client, requests := retryTestClient(t, func(w http.ResponseWriter, attempt int32) {
		if attempt == 1 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Write([]byte(`[]`))
	})

	start := time.Now()
	if _, err := client.ListBuckets(&BucketListOptions{}); err != nil {
		t.Fatal(err)
	}
	if got := requests.Load(); got != 2 {
		t.Fatalf("expected 2 requests, got %d", got)
	}
	if elapsed := time.Since(start); elapsed < time.Second {
		t.Fatalf("retried after %s; Retry-After: 1 was not honoured", elapsed)
	}
}

func TestRetryAfterBeyondMaxDelayIsNotRetried(t *testing.T) {
	client, requests := retryTestClient(t, func(w http.ResponseWriter, _ int32) {
		w.Header().Set("Retry-After", "60")
		w.WriteHeader(http.StatusTooManyRequests)
	})

	start := time.Now()
	_, err := client.ListBuckets(&BucketListOptions{})
	requireAPIStatus(t, err, http.StatusTooManyRequests)
	if got := requests.Load(); got != 1 {
		t.Fatalf("expected 1 request, got %d", got)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("returned after %s; expected no wait", elapsed)
	}
}

func TestTransient429IsRetriedWithBackoff(t *testing.T) {
	client, requests := retryTestClient(t, func(w http.ResponseWriter, _ int32) {
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"error":"too many outbound messages or recovery tasks are still pending; retry later"}`))
	})

	_, err := client.ListBuckets(&BucketListOptions{})
	requireAPIStatus(t, err, http.StatusTooManyRequests)
	if got := requests.Load(); got != 4 {
		t.Fatalf("expected 4 requests (1 + 3 retries), got %d", got)
	}
}

func TestServiceUnavailableIsRetriedPerPolicy(t *testing.T) {
	client, requests := retryTestClient(t, func(w http.ResponseWriter, _ int32) {
		w.WriteHeader(http.StatusServiceUnavailable)
	})

	_, err := client.ListBuckets(&BucketListOptions{})
	requireAPIStatus(t, err, http.StatusServiceUnavailable)
	if got := requests.Load(); got != 4 {
		t.Fatalf("expected 4 requests (1 + 3 retries), got %d", got)
	}
}
