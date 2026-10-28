package webhookrelay

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// errorServer answers every request with status, body and headers. Retries
// are disabled so 429/5xx come back on the first attempt.
func errorServer(t *testing.T, status int, body string, header http.Header) *API {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for k, v := range header {
			w.Header()[k] = v
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)
	api, err := NewWithAPIKey("sk-test", WithAPIEndpointURL(server.URL), WithRetryPolicy(0, 0, 0))
	if err != nil {
		t.Fatalf("NewWithAPIKey: %v", err)
	}
	return api
}

func TestAPIErrors(t *testing.T) {
	tests := []struct {
		name           string
		status         int
		body           string
		header         http.Header
		wantMessage    string
		wantRetryAfter time.Duration
		wantText       string
		is             func(error) bool
	}{
		{
			name: "400 with JSON body", status: http.StatusBadRequest,
			body:        `{"error":"invalid outbound request: url must use https"}`,
			wantMessage: "invalid outbound request: url must use https",
			wantText:    `{"error":"invalid outbound request: url must use https"}`,
			is:          IsBadRequest,
		},
		{
			name: "404", status: http.StatusNotFound,
			body:        `{"error":"consumer not found"}`,
			wantMessage: "consumer not found",
			wantText:    `HTTP status 404: content "{\"error\":\"consumer not found\"}"`,
			is:          IsNotFound,
		},
		{
			name: "409", status: http.StatusConflict,
			body:        `{"error":"idempotency key reused"}`,
			wantMessage: "idempotency key reused",
			wantText:    "HTTP status 409",
			is:          IsConflict,
		},
		{
			name: "429 with Retry-After", status: http.StatusTooManyRequests,
			body:           `{"error":"quota exceeded"}`,
			header:         http.Header{"Retry-After": []string{"7"}},
			wantMessage:    "quota exceeded",
			wantRetryAfter: 7 * time.Second,
			wantText:       "HTTP status 429",
			is:             IsRateLimited,
		},
		{
			name: "500 with plain body", status: http.StatusInternalServerError,
			body:     "boom",
			wantText: `HTTP status 500: content "boom"`,
			is:       func(err error) bool { return !IsNotFound(err) && !IsBadRequest(err) },
		},
		{
			name: "401", status: http.StatusUnauthorized,
			wantText: "HTTP status 401: invalid credentials",
			is:       func(err error) bool { return !IsNotFound(err) },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := errorServer(t, tt.status, tt.body, tt.header)
			// ListBuckets wraps the makeRequest error with errors.Wrap.
			_, err := api.ListBuckets(nil)
			var apiErr *APIError
			if !errors.As(err, &apiErr) {
				t.Fatalf("want an APIError through the wrap, got %T: %v", err, err)
			}
			if apiErr.StatusCode != tt.status {
				t.Errorf("StatusCode = %d, want %d", apiErr.StatusCode, tt.status)
			}
			if apiErr.Message != tt.wantMessage {
				t.Errorf("Message = %q, want %q", apiErr.Message, tt.wantMessage)
			}
			if apiErr.Body != tt.body {
				t.Errorf("Body = %q, want %q", apiErr.Body, tt.body)
			}
			if apiErr.RetryAfter != tt.wantRetryAfter {
				t.Errorf("RetryAfter = %s, want %s", apiErr.RetryAfter, tt.wantRetryAfter)
			}
			if !strings.Contains(err.Error(), tt.wantText) {
				t.Errorf("Error() = %q, want it to contain %q", err.Error(), tt.wantText)
			}
			if !tt.is(err) {
				t.Errorf("status helper did not match %v", err)
			}
		})
	}
}

func TestTransportErrorIsNotAPIError(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	url := server.URL
	server.Close()
	api, err := NewWithAPIKey("sk-test", WithAPIEndpointURL(url), WithRetryPolicy(0, 0, 0))
	if err != nil {
		t.Fatalf("NewWithAPIKey: %v", err)
	}
	_, err = api.ListBuckets(nil)
	if err == nil {
		t.Fatal("want an error from a closed server")
	}
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		t.Fatalf("transport error must not be an APIError: %v", err)
	}
	if IsNotFound(err) || IsRateLimited(err) {
		t.Fatalf("status helpers matched a transport error: %v", err)
	}
}

func TestParseRetryAfter(t *testing.T) {
	now := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		header string
		want   time.Duration
		ok     bool
	}{
		{"120", 2 * time.Minute, true},
		{now.Add(time.Hour).Format(http.TimeFormat), time.Hour, true},
		{now.Add(-time.Hour).Format(http.TimeFormat), 0, true},
		{"-1", 0, false},
		{"soon", 0, false},
	}
	for _, tt := range tests {
		got, ok := parseRetryAfter(tt.header, now)
		if got != tt.want || ok != tt.ok {
			t.Errorf("parseRetryAfter(%q) = %s, %v; want %s, %v", tt.header, got, ok, tt.want, tt.ok)
		}
	}
}
