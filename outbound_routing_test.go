package webhookrelay

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"
)

type routedRequest struct {
	Method, Path, Environment, IdempotencyKey string
	Body                                      map[string]interface{}
}

// routingServer answers every request with status and response and records
// the request, its environment header included.
func routingServer(t *testing.T, status int, response string, opts ...Option) (*API, *[]routedRequest) {
	t.Helper()
	var requests []routedRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		req := routedRequest{
			Method:         r.Method,
			Path:           r.URL.EscapedPath(),
			Environment:    r.Header.Get(OutboundEnvironmentHeader),
			IdempotencyKey: r.Header.Get("Idempotency-Key"),
		}
		if raw, _ := io.ReadAll(r.Body); len(raw) > 0 {
			if err := json.Unmarshal(raw, &req.Body); err != nil {
				t.Errorf("request body is not JSON: %s", raw)
			}
		}
		requests = append(requests, req)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(response))
	}))
	t.Cleanup(server.Close)
	api, err := NewWithAPIKey("sk-test", append([]Option{WithAPIEndpointURL(server.URL), WithRetryPolicy(0, 0, 0)}, opts...)...)
	if err != nil {
		t.Fatalf("NewWithAPIKey: %v", err)
	}
	return api, &requests
}

func mustJSON(t *testing.T, s string) map[string]interface{} {
	t.Helper()
	var out map[string]interface{}
	if err := json.Unmarshal([]byte(s), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestOutboundEndpointChannelsAndFilter(t *testing.T) {
	filter := OutboundFilter{
		"data.customer.tier": "gold",
		"amount":             map[string]interface{}{"$gte": 100, "$lt": 1000},
		"$or":                []interface{}{map[string]interface{}{"status": "paid"}, map[string]interface{}{"refund": map[string]interface{}{"$exists": false}}},
	}
	api, requests := routingServer(t, http.StatusCreated,
		`{"id":"ep-1","consumer":"cust_42","url":"https://example.com/hook","event_types":["*"],"channels":["eu"],"filter":{"data.customer.tier":"gold"},"environment":"test"}`)

	endpoint, err := api.CreateOutboundEndpoint(&OutboundEndpointOptions{
		Consumer:   "cust_42",
		URL:        "https://example.com/hook",
		EventTypes: []string{"*"},
		Channels:   []string{"eu", "project-123"},
		Filter:     filter,
	})
	if err != nil {
		t.Fatalf("CreateOutboundEndpoint: %v", err)
	}
	req := (*requests)[0]
	if req.Method != http.MethodPost || req.Path != "/outbound/consumers/cust_42/endpoints" {
		t.Fatalf("unexpected request %s %s", req.Method, req.Path)
	}
	want := mustJSON(t, `{"url":"https://example.com/hook","event_types":["*"],"channels":["eu","project-123"],
		"filter":{"data.customer.tier":"gold","amount":{"$gte":100,"$lt":1000},"$or":[{"status":"paid"},{"refund":{"$exists":false}}]}}`)
	if !reflect.DeepEqual(req.Body, want) {
		t.Fatalf("body = %v\nwant   %v", req.Body, want)
	}
	if endpoint.Environment != "test" || !reflect.DeepEqual(endpoint.Channels, []string{"eu"}) || endpoint.Filter["data.customer.tier"] != "gold" {
		t.Fatalf("unexpected endpoint %+v", endpoint)
	}
}

// An endpoint without channels or a filter sends neither: the API reads that
// as "every message".
func TestOutboundEndpointWithoutRoutingOmitsIt(t *testing.T) {
	api, requests := routingServer(t, http.StatusOK, `{"id":"ep-1"}`)
	if _, err := api.UpdateOutboundEndpoint("ep-1", &OutboundEndpointOptions{URL: "https://example.com/hook", EventTypes: []string{"*"}}); err != nil {
		t.Fatal(err)
	}
	body := (*requests)[0].Body
	if _, ok := body["channels"]; ok {
		t.Fatalf("channels sent: %v", body)
	}
	if _, ok := body["filter"]; ok {
		t.Fatalf("filter sent: %v", body)
	}
}

// UpdateOptions carries every setting, so a read-modify-write through the
// full-replace PUT keeps the channels and the filter.
func TestOutboundEndpointUpdateOptionsKeepsEverySetting(t *testing.T) {
	api, requests := routingServer(t, http.StatusOK, `{"id":"ep-1"}`)
	endpoint := &OutboundEndpoint{
		ID: "ep-1", Consumer: "cust_42", Type: OutboundEndpointTypeWebhook, URL: "https://example.com/hook",
		Description: "orders", EventTypes: []string{"invoice.paid"}, Headers: map[string]string{"X-Team": "billing"},
		Rate: 5, Timeout: 30, FunctionID: "fn-1", AutoDisable: true,
		Channels: []string{"eu"}, Filter: OutboundFilter{"type": "invoice"},
		State: OutboundEndpointStateActive, Secret: "whsec_never_sent",
	}
	options := endpoint.UpdateOptions()
	options.Description = "orders v2"
	if _, err := api.UpdateOutboundEndpoint(endpoint.ID, options); err != nil {
		t.Fatal(err)
	}
	req := (*requests)[0]
	if req.Method != http.MethodPut || req.Path != "/outbound/endpoints/ep-1" {
		t.Fatalf("unexpected request %s %s", req.Method, req.Path)
	}
	want := mustJSON(t, `{"type":"webhook","url":"https://example.com/hook","description":"orders v2","event_types":["invoice.paid"],
		"headers":{"X-Team":"billing"},"rate":5,"timeout":30,"auto_disable":true,"function_id":"fn-1","channels":["eu"],"filter":{"type":"invoice"}}`)
	if !reflect.DeepEqual(req.Body, want) {
		t.Fatalf("body = %v\nwant   %v", req.Body, want)
	}
	// The copy is independent of the endpoint it came from.
	options.Channels[0] = "us"
	options.Headers["X-Team"] = "ops"
	if endpoint.Channels[0] != "eu" || endpoint.Headers["X-Team"] != "billing" {
		t.Fatalf("UpdateOptions shares slices or maps with the endpoint: %+v", endpoint)
	}
}

// A cloud endpoint's URL is derived from its destination, so UpdateOptions
// sends the destination and no URL, and no credentials (the stored ones stay).
func TestOutboundCloudEndpointUpdateOptions(t *testing.T) {
	endpoint := &OutboundEndpoint{
		ID: "ep-1", Type: OutboundEndpointTypeGCPPubSub, URL: "pubsub://projects/p/topics/orders",
		Destination:         &OutboundDestination{GCPPubSub: &OutboundGCPPubSub{Topic: "projects/p/topics/orders"}},
		CredentialsIdentity: "sa@p.iam.gserviceaccount.com", EventTypes: []string{"*"},
	}
	options := endpoint.UpdateOptions()
	if options.URL != "" || options.Credentials != nil || options.Destination == nil || options.Type != OutboundEndpointTypeGCPPubSub {
		t.Fatalf("unexpected options %+v", options)
	}
}

func TestPublishOutboundMessageWithChannels(t *testing.T) {
	api, requests := routingServer(t, http.StatusAccepted,
		`{"id":"msg-1","consumer":"cust_42","event_type":"invoice.paid","endpoint_ids":["ep-1"],"skipped_endpoint_ids":["ep-2"],"skip_reasons":{"ep-2":"paused"},"channels":["eu"],"environment":"live"}`)
	message, err := api.PublishOutboundMessage(&OutboundMessagePublishOptions{
		Consumer: "cust_42", EventType: "invoice.paid", Payload: map[string]interface{}{"id": "inv_1"},
		Channels: []string{"eu", "project-123"}, IdempotencyKey: "inv_1",
	})
	if err != nil {
		t.Fatal(err)
	}
	req := (*requests)[0]
	want := mustJSON(t, `{"consumer":"cust_42","event_type":"invoice.paid","payload":{"id":"inv_1"},"channels":["eu","project-123"]}`)
	if req.Method != http.MethodPost || req.Path != "/outbound/messages" || req.IdempotencyKey != "inv_1" || !reflect.DeepEqual(req.Body, want) {
		t.Fatalf("unexpected request %+v", req)
	}
	if message.Environment != "live" || message.Channels[0] != "eu" || message.SkipReasons["ep-2"] != OutboundEndpointStatePaused {
		t.Fatalf("unexpected message %+v", message)
	}
}

func TestSendOutboundTest(t *testing.T) {
	t.Run("event type and payload", func(t *testing.T) {
		api, requests := routingServer(t, http.StatusAccepted, `{"id":"msg-1","endpoint_ids":["ep-1"],"test":true}`)
		message, err := api.SendOutboundTest("ep/1", &OutboundTestOptions{
			EventType: "invoice.paid", Payload: map[string]interface{}{"amount": 4900}, IdempotencyKey: "test-1",
		})
		if err != nil {
			t.Fatal(err)
		}
		req := (*requests)[0]
		want := mustJSON(t, `{"event_type":"invoice.paid","payload":{"amount":4900}}`)
		if req.Method != http.MethodPost || req.Path != "/outbound/endpoints/ep%2F1/test" || req.IdempotencyKey != "test-1" || !reflect.DeepEqual(req.Body, want) {
			t.Fatalf("unexpected request %+v", req)
		}
		if !message.Test {
			t.Fatalf("the receipt is marked test: %+v", message)
		}
	})
	t.Run("defaults", func(t *testing.T) {
		api, requests := routingServer(t, http.StatusAccepted, `{"id":"msg-1","test":true}`)
		if _, err := api.SendOutboundTest("ep-1", nil); err != nil {
			t.Fatal(err)
		}
		req := (*requests)[0]
		if len(req.Body) != 0 || len(req.IdempotencyKey) != 32 {
			t.Fatalf("expected an empty body and a generated idempotency key, got %+v", req)
		}
	})
	t.Run("paused endpoint is a 409", func(t *testing.T) {
		api, _ := routingServer(t, http.StatusConflict, `{"error":"endpoint is paused"}`)
		_, err := api.SendOutboundTest("ep-1", nil)
		if !IsConflict(err) {
			t.Fatalf("expected a 409 APIError, got %v", err)
		}
	})
}

func TestOutboundEnvironmentHeader(t *testing.T) {
	t.Run("no environment sends no header", func(t *testing.T) {
		api, requests := routingServer(t, http.StatusOK, `[]`)
		if _, err := api.ListOutboundConsumers(); err != nil {
			t.Fatal(err)
		}
		if env := (*requests)[0].Environment; env != "" {
			t.Fatalf("environment header = %q", env)
		}
	})
	t.Run("client option applies to every outbound call", func(t *testing.T) {
		api, requests := routingServer(t, http.StatusOK, `{"id":"x"}`, WithOutboundEnvironment("test"))
		_, _ = api.ListOutboundConsumers()
		_, _ = api.PublishOutboundMessage(&OutboundMessagePublishOptions{Consumer: "c", EventType: "e"})
		_, _ = api.GetOutboundImport("import-1")
		for _, req := range *requests {
			if req.Environment != "test" {
				t.Fatalf("%s %s environment = %q", req.Method, req.Path, req.Environment)
			}
		}
		if api.OutboundEnvironmentName() != "test" {
			t.Fatalf("OutboundEnvironmentName = %q", api.OutboundEnvironmentName())
		}
	})
	t.Run("derived client binds another environment and leaves the original", func(t *testing.T) {
		api, requests := routingServer(t, http.StatusOK, `[]`, WithOutboundEnvironment("test"))
		live := api.OutboundEnvironment("live")
		_, _ = live.ListOutboundEventTypes()
		_, _ = api.ListOutboundEventTypes()
		_, _ = api.OutboundEnvironment("").ListOutboundEventTypes()
		got := []string{(*requests)[0].Environment, (*requests)[1].Environment, (*requests)[2].Environment}
		if !reflect.DeepEqual(got, []string{"live", "test", ""}) {
			t.Fatalf("environments = %q", got)
		}
	})
	t.Run("unknown environment is a 404", func(t *testing.T) {
		api, _ := routingServer(t, http.StatusNotFound, `{"error":"environment staging not found"}`, WithOutboundEnvironment("staging"))
		_, err := api.ListOutboundConsumers()
		if !IsNotFound(err) {
			t.Fatalf("expected a 404, got %v", err)
		}
	})
}

func TestOutboundEnvironmentRequests(t *testing.T) {
	t.Run("list", func(t *testing.T) {
		api, requests := routingServer(t, http.StatusOK,
			`[{"name":"default","default":true,"created_at":"2026-11-01T00:00:00Z"},{"name":"test","description":"Staging","default":false,"created_at":"2026-11-02T00:00:00Z"}]`)
		environments, err := api.ListOutboundEnvironments()
		if err != nil {
			t.Fatal(err)
		}
		if req := (*requests)[0]; req.Method != http.MethodGet || req.Path != "/outbound/environments" {
			t.Fatalf("unexpected request %+v", req)
		}
		if len(environments) != 2 || !environments[0].Default || environments[0].Name != OutboundDefaultEnvironment || environments[1].Description != "Staging" {
			t.Fatalf("unexpected environments %+v", environments)
		}
	})
	t.Run("create", func(t *testing.T) {
		api, requests := routingServer(t, http.StatusCreated, `{"name":"test","description":"Staging","default":false,"created_at":"2026-11-02T00:00:00Z"}`)
		environment, err := api.CreateOutboundEnvironment(&OutboundEnvironmentOptions{Name: "test", Description: "Staging"})
		if err != nil {
			t.Fatal(err)
		}
		req := (*requests)[0]
		if req.Method != http.MethodPost || req.Path != "/outbound/environments" || !reflect.DeepEqual(req.Body, mustJSON(t, `{"name":"test","description":"Staging"}`)) {
			t.Fatalf("unexpected request %+v", req)
		}
		if environment.Name != "test" || environment.CreatedAt.IsZero() {
			t.Fatalf("unexpected environment %+v", environment)
		}
	})
	t.Run("create existing is a 409", func(t *testing.T) {
		api, _ := routingServer(t, http.StatusConflict, `{"error":"environment test already exists"}`)
		_, err := api.CreateOutboundEnvironment(&OutboundEnvironmentOptions{Name: "test"})
		if !IsConflict(err) {
			t.Fatalf("expected a 409, got %v", err)
		}
	})
	t.Run("delete", func(t *testing.T) {
		api, requests := routingServer(t, http.StatusOK, `{"deleted":true}`)
		if err := api.DeleteOutboundEnvironment("test"); err != nil {
			t.Fatal(err)
		}
		if req := (*requests)[0]; req.Method != http.MethodDelete || req.Path != "/outbound/environments/test" {
			t.Fatalf("unexpected request %+v", req)
		}
	})
}

func TestOutboundPortalSessions(t *testing.T) {
	t.Run("create", func(t *testing.T) {
		api, requests := routingServer(t, http.StatusCreated,
			`{"url":"https://my.webhookrelay.com/portal/p1#token=wrpt_x","expires_at":"2026-11-24T00:00:00Z","token_expires_at":"2026-11-23T00:15:00Z"}`,
			WithOutboundEnvironment("live"))
		link, err := api.CreateOutboundPortalSession("customer:42", &OutboundPortalSessionOptions{
			Capabilities: OutboundPortalRead, TTL: 2 * time.Hour, Next: "endpoints/new", SessionID: "user-1",
		})
		if err != nil {
			t.Fatal(err)
		}
		req := (*requests)[0]
		want := mustJSON(t, `{"capabilities":"read","ttl":7200,"next":"endpoints/new","session_id":"user-1"}`)
		if req.Method != http.MethodPost || req.Path != "/outbound/consumers/customer:42/portal-sessions" || req.Environment != "live" || !reflect.DeepEqual(req.Body, want) {
			t.Fatalf("unexpected request %+v", req)
		}
		if link.URL != "https://my.webhookrelay.com/portal/p1#token=wrpt_x" ||
			!link.ExpiresAt.Equal(time.Date(2026, 11, 24, 0, 0, 0, 0, time.UTC)) ||
			!link.TokenExpiresAt.Equal(time.Date(2026, 11, 23, 0, 15, 0, 0, time.UTC)) {
			t.Fatalf("unexpected link %+v", link)
		}
	})
	t.Run("create with defaults", func(t *testing.T) {
		api, requests := routingServer(t, http.StatusCreated, `{"url":"u"}`)
		if _, err := api.CreateOutboundPortalSession("cust_42", nil); err != nil {
			t.Fatal(err)
		}
		if body := (*requests)[0].Body; len(body) != 0 {
			t.Fatalf("expected an empty body, got %v", body)
		}
	})
	t.Run("expire tagged sessions", func(t *testing.T) {
		api, requests := routingServer(t, http.StatusOK, `{"ended":2}`)
		ended, err := api.ExpireOutboundPortalSessions("cust_42", "user-1")
		if err != nil {
			t.Fatal(err)
		}
		req := (*requests)[0]
		if req.Method != http.MethodPost || req.Path != "/outbound/consumers/cust_42/portal-sessions/expire" || !reflect.DeepEqual(req.Body, mustJSON(t, `{"session_id":"user-1"}`)) {
			t.Fatalf("unexpected request %+v", req)
		}
		if ended != 2 {
			t.Fatalf("ended = %d", ended)
		}
	})
	t.Run("expire all sessions", func(t *testing.T) {
		api, requests := routingServer(t, http.StatusOK, `{"ended":0}`)
		if _, err := api.ExpireOutboundPortalSessions("cust_42", ""); err != nil {
			t.Fatal(err)
		}
		if body := (*requests)[0].Body; len(body) != 0 {
			t.Fatalf("expected an empty body, got %v", body)
		}
	})
}
