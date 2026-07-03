//go:generate jsonenums -type=RequestStatus
package webhookrelay

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestListWebhookLogsAppliesQuery verifies the filter/pagination options reach
// the request as query parameters (a regression guard: they were previously
// built but never attached to the URL).
func TestListWebhookLogsAppliesQuery(t *testing.T) {
	var gotPath, gotQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotQuery = r.URL.Path, r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[],"total":0,"limit":50,"offset":0}`))
	}))
	defer server.Close()

	client, err := New("k", "s", WithAPIEndpointURL(server.URL))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if _, err := client.ListWebhookLogs(&WebhookLogsListOptions{
		BucketID: "bucket-1",
		Status:   RequestStatusSent,
		Limit:    50,
	}); err != nil {
		t.Fatalf("ListWebhookLogs: %v", err)
	}

	if gotPath != "/logs" {
		t.Fatalf("unexpected path: %s", gotPath)
	}
	for _, want := range []string{"bucket=bucket-1", "status=sent", "limit=50"} {
		if !strings.Contains(gotQuery, want) {
			t.Errorf("query %q missing %q", gotQuery, want)
		}
	}
}

// TestListWebhookLogsRequiresBucket guards the client-side validation.
func TestListWebhookLogsRequiresBucket(t *testing.T) {
	client, err := New("k", "s")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := client.ListWebhookLogs(&WebhookLogsListOptions{}); err == nil {
		t.Fatal("expected error when BucketID is empty")
	}
}

func Test_getQuery(t *testing.T) {
	type args struct {
		options *WebhookLogsListOptions
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "nothing",
			args: args{options: &WebhookLogsListOptions{}},
			want: "",
		},
		{
			name: "limit",
			args: args{options: &WebhookLogsListOptions{
				Limit: 100,
			}},
			want: "limit=100",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := getQuery(tt.args.options); got != tt.want {
				t.Errorf("getQuery() = %v, want %v", got, tt.want)
			}
		})
	}
}
