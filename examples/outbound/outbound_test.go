package outbound

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/webhookrelay/webhookrelay-go"
)

func TestExampleOnboardsAndPublishes(t *testing.T) {
	type request struct {
		method, path, idempotencyKey string
		body                         map[string]interface{}
	}
	var requests []request
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		req := request{method: r.Method, path: r.URL.Path, idempotencyKey: r.Header.Get("Idempotency-Key")}
		_ = json.NewDecoder(r.Body).Decode(&req.body)
		requests = append(requests, req)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"resource-1","secret":"whsec_abc"}`))
	}))
	defer server.Close()

	api, err := webhookrelay.NewWithAPIKey("sk-test", webhookrelay.WithAPIEndpointURL(server.URL))
	if err != nil {
		t.Fatal(err)
	}
	endpoint, err := OnboardCustomer(api, "https://customer.example/webhooks")
	if err != nil {
		t.Fatalf("OnboardCustomer: %v", err)
	}
	if endpoint.Secret != "whsec_abc" {
		t.Fatalf("the new endpoint carries its signing secret, got %+v", endpoint)
	}
	if _, err := PublishInvoicePaid(api); err != nil {
		t.Fatalf("PublishInvoicePaid: %v", err)
	}

	want := []string{
		"PUT /outbound/event-types/invoice.paid",
		"PUT /outbound/consumers/customer_42",
		"POST /outbound/consumers/customer_42/endpoints",
		"POST /outbound/messages",
	}
	if len(requests) != len(want) {
		t.Fatalf("requests = %+v", requests)
	}
	for i, req := range requests {
		if got := req.method + " " + req.path; got != want[i] {
			t.Fatalf("request %d = %s, want %s", i, got, want[i])
		}
	}
	if url := requests[2].body["url"]; url != "https://customer.example/webhooks" {
		t.Fatalf("endpoint url = %v", url)
	}
	if key := requests[3].idempotencyKey; key != "invoice-paid:inv_123" {
		t.Fatalf("idempotency key = %q", key)
	}
}

func TestExampleFindsFailingEndpoints(t *testing.T) {
	var requests []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.Method+" "+r.URL.RequestURI())
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/outbound/health" {
			_, _ = w.Write([]byte(`{"endpoints":{"active":1,"failing":1,"paused":0,"disabled":0},"stats":{"attempts":4,"failures":3},"since":"2026-09-27T12:00:00Z"}`))
			return
		}
		_, _ = w.Write([]byte(`[{"id":"endpoint-1","consumer":"customer_42","state":"failing","stats":{"attempts":4,"failures":3}}]`))
	}))
	defer server.Close()

	api, err := webhookrelay.NewWithAPIKey("sk-test", webhookrelay.WithAPIEndpointURL(server.URL))
	if err != nil {
		t.Fatal(err)
	}
	failing, err := FailingEndpoints(api)
	if err != nil {
		t.Fatalf("FailingEndpoints: %v", err)
	}
	if len(failing) != 1 || failing[0].Consumer != "customer_42" || failing[0].Stats == nil || failing[0].Stats.Failures != 3 {
		t.Fatalf("failing = %+v", failing)
	}
	want := []string{"GET /outbound/health", "GET /outbound/endpoints?limit=20&state=failing"}
	if len(requests) != len(want) || requests[0] != want[0] || requests[1] != want[1] {
		t.Fatalf("requests = %v, want %v", requests, want)
	}
}

func TestNewClientReadsTheAPIKeyFromTheEnvironment(t *testing.T) {
	t.Setenv("RELAY_API_KEY", "")
	if _, err := NewClient(); err == nil {
		t.Fatal("expected an error without RELAY_API_KEY")
	}
	t.Setenv("RELAY_API_KEY", "sk-test")
	api, err := NewClient()
	if err != nil || api.APIToken != "sk-test" {
		t.Fatalf("NewClient() = %+v, %v", api, err)
	}
}
