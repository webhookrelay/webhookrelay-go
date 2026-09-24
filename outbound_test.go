package webhookrelay

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type recordedRequest struct {
	Method, Path, Query, IdempotencyKey string
	Body                                map[string]interface{}
}

// outboundServer answers every request with response and records it.
func outboundServer(t *testing.T, response string) (*API, *[]recordedRequest) {
	t.Helper()
	var requests []recordedRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		req := recordedRequest{Method: r.Method, Path: r.URL.EscapedPath(), Query: r.URL.RawQuery, IdempotencyKey: r.Header.Get("Idempotency-Key")}
		if raw, _ := io.ReadAll(r.Body); len(raw) > 0 {
			if err := json.Unmarshal(raw, &req.Body); err != nil {
				t.Errorf("request body is not JSON: %s", raw)
			}
		}
		requests = append(requests, req)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(response))
	}))
	t.Cleanup(server.Close)
	api, err := NewWithAPIKey("sk-test", WithAPIEndpointURL(server.URL))
	if err != nil {
		t.Fatalf("NewWithAPIKey: %v", err)
	}
	return api, &requests
}

func TestOutboundRequests(t *testing.T) {
	since := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		name      string
		call      func(api *API) error
		method    string
		path      string
		query     string
		wantBody  map[string]interface{}
		checkBody bool
	}{
		{
			name: "upsert consumer",
			call: func(api *API) error {
				_, err := api.UpsertOutboundConsumer(&OutboundConsumerOptions{ID: "customer:42", Name: "Acme"})
				return err
			},
			method: http.MethodPut, path: "/outbound/consumers/customer:42",
			wantBody: map[string]interface{}{"name": "Acme"}, checkBody: true,
		},
		{
			name: "create endpoint",
			call: func(api *API) error {
				_, err := api.CreateOutboundEndpoint(&OutboundEndpointOptions{Consumer: "customer_42", URL: "https://example.com/hook", EventTypes: []string{"*"}})
				return err
			},
			method: http.MethodPost, path: "/outbound/consumers/customer_42/endpoints",
			wantBody: map[string]interface{}{"url": "https://example.com/hook", "event_types": []interface{}{"*"}}, checkBody: true,
		},
		{
			name:   "event type names are escaped",
			call:   func(api *API) error { return api.DeleteOutboundEventType("invoice/paid") },
			method: http.MethodDelete, path: "/outbound/event-types/invoice%2Fpaid",
		},
		{
			name: "pause endpoint",
			call: func(api *API) error {
				_, err := api.PauseOutboundEndpoint("endpoint-1")
				return err
			},
			method: http.MethodPost, path: "/outbound/endpoints/endpoint-1/pause",
		},
		{
			name: "list messages",
			call: func(api *API) error {
				_, err := api.ListOutboundMessages(&OutboundMessageListOptions{Consumer: "customer_42", Limit: 10, Offset: 20})
				return err
			},
			method: http.MethodGet, path: "/outbound/messages", query: "consumer=customer_42&limit=10&offset=20",
		},
		{
			name: "list deliveries",
			call: func(api *API) error {
				_, err := api.ListOutboundDeliveries(&OutboundDeliveryListOptions{EndpointID: "endpoint-1", Limit: 5})
				return err
			},
			method: http.MethodGet, path: "/outbound/endpoints/endpoint-1/deliveries", query: "limit=5",
		},
		{
			name: "retry delivery",
			call: func(api *API) error {
				_, err := api.RetryOutboundDelivery("endpoint-1", "message-1")
				return err
			},
			method: http.MethodPost, path: "/outbound/endpoints/endpoint-1/retry",
			wantBody: map[string]interface{}{"message_id": "message-1"}, checkBody: true,
		},
		{
			name: "recover deliveries since a time",
			call: func(api *API) error {
				_, err := api.RecoverOutboundDeliveries("endpoint-1", since)
				return err
			},
			method: http.MethodPost, path: "/outbound/endpoints/endpoint-1/recover",
			wantBody: map[string]interface{}{"since": "2026-09-01T00:00:00Z"}, checkBody: true,
		},
		{
			name: "replay missing defaults the window",
			call: func(api *API) error {
				_, err := api.ReplayMissingOutboundDeliveries("endpoint-1", time.Time{})
				return err
			},
			method: http.MethodPost, path: "/outbound/endpoints/endpoint-1/replay-missing",
			wantBody: map[string]interface{}{}, checkBody: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response := `{}`
			if tt.method == http.MethodGet {
				response = `[]`
			}
			api, requests := outboundServer(t, response)
			if err := tt.call(api); err != nil {
				t.Fatalf("call: %v", err)
			}
			if len(*requests) != 1 {
				t.Fatalf("expected one request, got %d", len(*requests))
			}
			got := (*requests)[0]
			if got.Method != tt.method || got.Path != tt.path || got.Query != tt.query {
				t.Fatalf("got %s %s?%s, want %s %s?%s", got.Method, got.Path, got.Query, tt.method, tt.path, tt.query)
			}
			if tt.checkBody && !jsonEqual(got.Body, tt.wantBody) {
				t.Fatalf("body = %v, want %v", got.Body, tt.wantBody)
			}
		})
	}
}

func TestPublishOutboundMessageIdempotencyKey(t *testing.T) {
	api, requests := outboundServer(t, `{"id":"message-1","endpoint_ids":["endpoint-1"]}`)
	publish := &OutboundMessagePublishOptions{Consumer: "customer_42", EventType: "invoice.paid", Payload: map[string]int{"amount": 4900}}

	message, err := api.PublishOutboundMessage(publish)
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	if message.ID != "message-1" || len(message.EndpointIDs) != 1 {
		t.Fatalf("unexpected message: %+v", message)
	}
	if _, err := api.PublishOutboundMessage(publish); err != nil {
		t.Fatalf("publish: %v", err)
	}
	first, second := (*requests)[0].IdempotencyKey, (*requests)[1].IdempotencyKey
	if first == "" || first == second {
		t.Fatalf("each publish without a key needs its own generated key, got %q and %q", first, second)
	}

	publish.IdempotencyKey = "invoice-paid:inv_123"
	if _, err := api.PublishOutboundMessage(publish); err != nil {
		t.Fatalf("publish: %v", err)
	}
	if got := (*requests)[2].IdempotencyKey; got != "invoice-paid:inv_123" {
		t.Fatalf("idempotency key = %q", got)
	}
	if body := (*requests)[2].Body; body["event_type"] != "invoice.paid" || body["consumer"] != "customer_42" {
		t.Fatalf("unexpected body: %v", body)
	}
}

func TestGetOutboundMessageDecodesDeliveries(t *testing.T) {
	api, _ := outboundServer(t, `{
		"id": "message-1",
		"consumer": "customer_42",
		"event_type": "invoice.paid",
		"payload": {"amount": 4900},
		"endpoint_ids": ["endpoint-1"],
		"created_at": "2026-09-01T10:00:00Z",
		"deliveries": [{"id": "delivery-1", "message_id": "message-1", "output_id": "endpoint-1", "status": "sent", "status_code": 204, "created_at": 1788256800}]
	}`)
	message, err := api.GetOutboundMessage("message-1")
	if err != nil {
		t.Fatalf("GetOutboundMessage: %v", err)
	}
	if string(message.Payload) != `{"amount": 4900}` {
		t.Fatalf("payload = %s", message.Payload)
	}
	if len(message.Deliveries) != 1 {
		t.Fatalf("deliveries = %+v", message.Deliveries)
	}
	delivery := message.Deliveries[0]
	if delivery.MessageID != "message-1" || delivery.Status != RequestStatusSent || delivery.CreatedAt.Unix() != 1788256800 {
		t.Fatalf("unexpected delivery: %+v", delivery)
	}
}

func TestRevealOutboundEndpointSecret(t *testing.T) {
	api, requests := outboundServer(t, `{"secret":"whsec_abc"}`)
	secret, err := api.RevealOutboundEndpointSecret("endpoint-1")
	if err != nil || secret != "whsec_abc" {
		t.Fatalf("secret = %q, err = %v", secret, err)
	}
	if got := (*requests)[0]; got.Method != http.MethodPost || got.Path != "/outbound/endpoints/endpoint-1/secret/reveal" {
		t.Fatalf("unexpected request %s %s", got.Method, got.Path)
	}
}

func jsonEqual(a, b interface{}) bool {
	ja, _ := json.Marshal(a)
	jb, _ := json.Marshal(b)
	return string(ja) == string(jb)
}
