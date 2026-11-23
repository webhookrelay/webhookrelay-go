package outbound

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/webhookrelay/webhookrelay-go"
)

type exampleRequest struct {
	method, path, environment string
	body                      map[string]interface{}
}

// exampleServer answers each request with the response for its "METHOD path"
// (status 200 unless given) and records it.
func exampleServer(t *testing.T, responses map[string]string, statuses map[string]int) (*webhookrelay.API, *[]exampleRequest) {
	t.Helper()
	var requests []exampleRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		req := exampleRequest{method: r.Method, path: r.URL.Path, environment: r.Header.Get(webhookrelay.OutboundEnvironmentHeader)}
		_ = json.NewDecoder(r.Body).Decode(&req.body)
		requests = append(requests, req)
		key := r.Method + " " + r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		if status, ok := statuses[key]; ok {
			w.WriteHeader(status)
		}
		response, ok := responses[key]
		if !ok {
			t.Errorf("unexpected request %s", key)
			response = `{}`
		}
		_, _ = w.Write([]byte(response))
	}))
	t.Cleanup(server.Close)
	api, err := webhookrelay.NewWithAPIKey("sk-test", webhookrelay.WithAPIEndpointURL(server.URL), webhookrelay.WithRetryPolicy(0, 0, 0))
	if err != nil {
		t.Fatal(err)
	}
	return api, &requests
}

func TestExampleRoutesByChannelAndFilter(t *testing.T) {
	api, requests := exampleServer(t, map[string]string{
		"POST /outbound/consumers/customer_42/endpoints": `{"id":"ep-1","channels":["eu"],"filter":{"amount":{"$gte":100}}}`,
		"POST /outbound/messages":                        `{"id":"msg-1","endpoint_ids":["ep-1"],"channels":["eu"]}`,
	}, nil)

	endpoint, err := CreateRegionalEndpoint(api, "https://customer.example/webhooks")
	if err != nil {
		t.Fatalf("CreateRegionalEndpoint: %v", err)
	}
	if !reflect.DeepEqual(endpoint.Channels, []string{"eu"}) || endpoint.Filter == nil {
		t.Fatalf("unexpected endpoint %+v", endpoint)
	}
	message, err := PublishToRegion(api)
	if err != nil {
		t.Fatalf("PublishToRegion: %v", err)
	}
	if !reflect.DeepEqual(message.EndpointIDs, []string{"ep-1"}) {
		t.Fatalf("unexpected message %+v", message)
	}

	created, published := (*requests)[0].body, (*requests)[1].body
	wantFilter := map[string]interface{}{"amount": map[string]interface{}{"$gte": float64(100)}, "refund": map[string]interface{}{"$exists": false}}
	if !reflect.DeepEqual(created["channels"], []interface{}{"eu"}) || !reflect.DeepEqual(created["filter"], wantFilter) {
		t.Fatalf("endpoint body = %v", created)
	}
	if !reflect.DeepEqual(published["channels"], []interface{}{"eu"}) {
		t.Fatalf("publish body = %v", published)
	}
}

func TestExampleUpdateKeepsTheFilter(t *testing.T) {
	api, requests := exampleServer(t, map[string]string{
		"GET /outbound/endpoints/ep-1": `{"id":"ep-1","consumer":"customer_42","url":"https://customer.example/webhooks","event_types":["invoice.paid"],"channels":["eu"],"filter":{"amount":{"$gte":100}}}`,
		"PUT /outbound/endpoints/ep-1": `{"id":"ep-1","channels":["us"],"filter":{"amount":{"$gte":100}}}`,
	}, nil)

	if _, err := MoveEndpointToRegion(api, "ep-1", "us"); err != nil {
		t.Fatalf("MoveEndpointToRegion: %v", err)
	}
	put := (*requests)[1].body
	if !reflect.DeepEqual(put["channels"], []interface{}{"us"}) || put["filter"] == nil || put["url"] != "https://customer.example/webhooks" {
		t.Fatalf("the update must carry every setting, got %v", put)
	}
}

func TestExampleSendsATestEvent(t *testing.T) {
	api, requests := exampleServer(t, map[string]string{
		"POST /outbound/endpoints/ep-1/test": `{"id":"msg-1","endpoint_ids":["ep-1"],"test":true}`,
	}, map[string]int{"POST /outbound/endpoints/ep-1/test": http.StatusAccepted})

	message, err := SendTestEvent(api, "ep-1")
	if err != nil {
		t.Fatalf("SendTestEvent: %v", err)
	}
	if !message.Test || (*requests)[0].body["event_type"] != "invoice.paid" {
		t.Fatalf("unexpected test send %+v, %v", message, (*requests)[0].body)
	}
}

func TestExampleSetsUpATestEnvironment(t *testing.T) {
	for _, status := range []int{http.StatusCreated, http.StatusConflict} {
		api, requests := exampleServer(t, map[string]string{
			"POST /outbound/environments": `{"name":"test"}`,
			"GET /outbound/consumers":     `[]`,
		}, map[string]int{"POST /outbound/environments": status})

		test, err := SetUpTestEnvironment(api)
		if err != nil {
			t.Fatalf("status %d: SetUpTestEnvironment: %v", status, err)
		}
		_, _ = test.ListOutboundConsumers()
		_, _ = api.ListOutboundConsumers()
		got := []string{(*requests)[1].environment, (*requests)[2].environment}
		if !reflect.DeepEqual(got, []string{"test", ""}) {
			t.Fatalf("status %d: environments = %q, want the derived client in test and the original in default", status, got)
		}
	}
}

func TestExampleMintsAndEndsPortalSessions(t *testing.T) {
	api, requests := exampleServer(t, map[string]string{
		"POST /outbound/consumers/customer_42/portal-sessions":        `{"url":"https://my.webhookrelay.com/portal/p1#token=wrpt_x","expires_at":"2026-11-23T13:00:00Z","token_expires_at":"2026-11-23T12:15:00Z"}`,
		"POST /outbound/consumers/customer_42/portal-sessions/expire": `{"ended":1}`,
	}, map[string]int{"POST /outbound/consumers/customer_42/portal-sessions": http.StatusCreated})

	url, err := PortalURL(api, "user-1")
	if err != nil || url != "https://my.webhookrelay.com/portal/p1#token=wrpt_x" {
		t.Fatalf("PortalURL = %q, %v", url, err)
	}
	ended, err := SignOut(api, "user-1")
	if err != nil || ended != 1 {
		t.Fatalf("SignOut = %d, %v", ended, err)
	}
	minted := (*requests)[0].body
	if minted["ttl"] != float64(3600) || minted["session_id"] != "user-1" {
		t.Fatalf("portal session body = %v", minted)
	}
	if (*requests)[1].body["session_id"] != "user-1" {
		t.Fatalf("expire body = %v", (*requests)[1].body)
	}
}
