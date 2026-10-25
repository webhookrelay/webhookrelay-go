package webhookrelay

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"
)

// TestIntegrationOutbound sends outbound webhooks end to end against the live
// API: event type + consumer + endpoints pointed at free Webhook Bins, signed
// deliveries verified with the Standard Webhooks scheme, idempotent publishing,
// delivery history filters, secret import, health, retry and secret rotation.
// Everything it creates is deleted on exit, even when it fails.
func TestIntegrationOutbound(t *testing.T) {
	client := integrationClient(t)

	// Bound the whole flow so a stuck delivery cannot hang CI. Cleanup uses
	// the unbounded client so it still runs after the deadline.
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Second)
	defer cancel()
	api := client.WithContext(ctx)

	access, err := api.GetOutboundAccess()
	if err != nil {
		t.Fatalf("GetOutboundAccess: %v", err)
	}
	if !access.Enabled {
		plan := "unknown"
		if info, err := api.UserInfo(); err == nil && info.Data.PlanID != "" {
			plan = info.Data.PlanID
		}
		request := "none"
		if access.Request != nil {
			request = "ticket " + access.Request.TicketID + " opened " + access.Request.RequestedAt.Format(time.RFC3339)
		}
		t.Fatalf("outbound not enabled for the CI account (needs Basic+ plan); GET /v1/outbound/access returned enabled=false (plan %s, open access request: %s)", plan, request)
	}
	if access.Limits != nil {
		t.Logf("outbound limits: %d consumers, %d endpoints per consumer, %d event types",
			access.Limits.Consumers, access.Limits.EndpointsPerConsumer, access.Limits.EventTypes)
	}

	bins := &webhookBin{http: &http.Client{Timeout: 15 * time.Second}}
	bin1 := bins.create(ctx, t)
	bin2 := bins.create(ctx, t)

	name := fmt.Sprintf("whr-go-sdk-%d-%s", time.Now().Unix(), randomHex(t, 4))
	cleanup := &outboundCleanup{client: client}
	t.Cleanup(func() { cleanup.run(t) })

	// --- Event type + consumer ---
	eventType, err := api.UpsertOutboundEventType(&OutboundEventTypeOptions{
		Name:        name,
		Description: "Go SDK live test event",
		Example:     map[string]interface{}{"order_id": "ord_1"},
	})
	if err != nil {
		t.Fatalf("UpsertOutboundEventType: %v", err)
	}
	cleanup.eventType = name
	if eventType.Name != name || eventType.Deprecated {
		t.Fatalf("unexpected event type: %+v", eventType)
	}

	consumer, err := api.UpsertOutboundConsumer(&OutboundConsumerOptions{ID: name, Name: "Go SDK live test"})
	if err != nil {
		t.Fatalf("UpsertOutboundConsumer: %v", err)
	}
	cleanup.consumer = name
	if consumer.ID != name || consumer.Name != "Go SDK live test" {
		t.Fatalf("unexpected consumer: %+v", consumer)
	}
	if got, err := api.GetOutboundConsumer(name); err != nil || got.ID != name {
		t.Fatalf("GetOutboundConsumer = %+v, %v", got, err)
	}

	// --- Endpoint with a generated secret ---
	ep1, err := api.CreateOutboundEndpoint(&OutboundEndpointOptions{
		Consumer:    name,
		URL:         bin1.url(),
		EventTypes:  []string{name},
		Description: "generated secret",
	})
	if err != nil {
		t.Fatalf("CreateOutboundEndpoint: %v", err)
	}
	cleanup.endpoints = append(cleanup.endpoints, ep1.ID)
	if ep1.ID == "" || ep1.Consumer != name || ep1.URL != bin1.url() || ep1.State != OutboundEndpointStateActive {
		t.Fatalf("unexpected endpoint: %s", describeEndpoint(ep1))
	}
	if !strings.HasPrefix(ep1.Secret, "whsec_") {
		t.Fatalf("endpoint secret does not have the whsec_ prefix")
	}
	secret1 := ep1.Secret

	fetched, err := api.GetOutboundEndpoint(ep1.ID)
	if err != nil {
		t.Fatalf("GetOutboundEndpoint: %v", err)
	}
	if fetched.ID != ep1.ID || fetched.URL != bin1.url() || fetched.Secret != "" {
		t.Fatalf("GetOutboundEndpoint must return the endpoint without its secret: %s (secret set: %t)", describeEndpoint(fetched), fetched.Secret != "")
	}
	listed, err := api.ListOutboundEndpoints(name)
	if err != nil {
		t.Fatalf("ListOutboundEndpoints: %v", err)
	}
	if findEndpoint(listed, ep1.ID) == nil {
		t.Fatalf("endpoint %s missing from ListOutboundEndpoints", ep1.ID)
	}

	// --- Publish, idempotently ---
	payload := map[string]interface{}{"order_id": name, "amount": 4200, "currency": "EUR", "paid": true}
	publish := &OutboundMessagePublishOptions{
		Consumer:       name,
		EventType:      name,
		EventID:        name + "-evt-1",
		Payload:        payload,
		IdempotencyKey: name + "-publish-1",
	}
	msg1, err := api.PublishOutboundMessage(publish)
	if err != nil {
		t.Fatalf("PublishOutboundMessage: %v", err)
	}
	if msg1.ID == "" || msg1.Consumer != name || msg1.EventType != name {
		t.Fatalf("unexpected message receipt: id=%q consumer=%q event_type=%q", msg1.ID, msg1.Consumer, msg1.EventType)
	}
	again, err := api.PublishOutboundMessage(publish)
	if err != nil {
		t.Fatalf("PublishOutboundMessage (same idempotency key): %v", err)
	}
	if again.ID != msg1.ID {
		t.Fatalf("republishing with the same idempotency key returned message %s, want %s", again.ID, msg1.ID)
	}

	// --- The signed delivery arrives at the bin ---
	delivery := bin1.waitFor(ctx, t, msg1.ID, 1)
	verifyDelivery(t, delivery, msg1.ID, name, payload, secret1)

	// --- Delivery history ---
	sent := waitForSent(ctx, t, api, msg1.ID, ep1.ID)
	if sent.MessageID != msg1.ID || sent.EventType != name || sent.StatusCode != http.StatusOK {
		t.Fatalf("unexpected delivery: message=%s event_type=%s status_code=%d", sent.MessageID, sent.EventType, sent.StatusCode)
	}
	message, err := api.GetOutboundMessage(msg1.ID)
	if err != nil {
		t.Fatalf("GetOutboundMessage: %v", err)
	}
	if message.EventID != publish.EventID || !containsString(message.EndpointIDs, ep1.ID) || !jsonBytesEqual(t, message.Payload, payload) {
		t.Fatalf("unexpected message: event_id=%q endpoint_ids=%v payload=%s", message.EventID, message.EndpointIDs, message.Payload)
	}
	// The idempotent republish must not have produced a second delivery.
	if n := len(bin1.requestsFor(ctx, t, msg1.ID)); n != 1 {
		t.Fatalf("bin received %d deliveries of message %s, want 1", n, msg1.ID)
	}

	messages, err := api.ListOutboundMessages(&OutboundMessageListOptions{Consumer: name, EventType: name})
	if err != nil {
		t.Fatalf("ListOutboundMessages: %v", err)
	}
	if !containsMessage(messages, msg1.ID) {
		t.Fatalf("message %s missing from ListOutboundMessages", msg1.ID)
	}

	listDeliveries := func(opts OutboundDeliveryListOptions) []*Log {
		t.Helper()
		opts.EndpointID = ep1.ID
		logs, err := api.ListOutboundDeliveries(&opts)
		if err != nil {
			t.Fatalf("ListOutboundDeliveries(%+v): %v", opts, err)
		}
		return logs
	}
	eventually(ctx, t, 30*time.Second, "delivery listed by status=sent and event type", func() bool {
		return containsDelivery(listDeliveries(OutboundDeliveryListOptions{Status: "sent"}), msg1.ID) &&
			containsDelivery(listDeliveries(OutboundDeliveryListOptions{EventType: name}), msg1.ID)
	})
	byMessage := listDeliveries(OutboundDeliveryListOptions{MessageID: msg1.ID})
	if len(byMessage) != 1 || byMessage[0].MessageID != msg1.ID || byMessage[0].Status != RequestStatusSent {
		t.Fatalf("ListOutboundDeliveries by message returned %d deliveries", len(byMessage))
	}
	if containsDelivery(listDeliveries(OutboundDeliveryListOptions{Status: "failed"}), msg1.ID) {
		t.Fatalf("sent delivery listed under status=failed")
	}
	if n := len(listDeliveries(OutboundDeliveryListOptions{MessageID: msg1.ID, Status: "failed"})); n != 0 {
		t.Fatalf("message+status=failed filter returned %d deliveries, want 0", n)
	}
	if n := len(listDeliveries(OutboundDeliveryListOptions{EventType: name + "-other"})); n != 0 {
		t.Fatalf("unknown event type filter returned %d deliveries, want 0", n)
	}

	// --- Secret import ---
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("rand: %v", err)
	}
	imported := "whsec_" + base64.StdEncoding.EncodeToString(key)
	ep2, err := api.CreateOutboundEndpoint(&OutboundEndpointOptions{
		Consumer:    name,
		URL:         bin2.url(),
		EventTypes:  []string{name},
		Description: "imported secret",
		Secret:      imported,
	})
	if err != nil {
		t.Fatalf("CreateOutboundEndpoint with an imported secret: %s", redact(err.Error(), imported))
	}
	cleanup.endpoints = append(cleanup.endpoints, ep2.ID)
	if ep2.Secret != imported {
		t.Fatalf("CreateOutboundEndpoint returned a different secret than the imported one")
	}
	if revealed, err := api.RevealOutboundEndpointSecret(ep2.ID); err != nil || revealed != imported {
		t.Fatalf("RevealOutboundEndpointSecret: matches imported=%t, err=%v", revealed == imported, err)
	}

	const invalidSecret = "whsec_short"
	bad, err := api.CreateOutboundEndpoint(&OutboundEndpointOptions{
		Consumer:   name,
		URL:        bin2.url() + "?invalid-secret",
		EventTypes: []string{name},
		Secret:     invalidSecret,
	})
	if err == nil {
		cleanup.endpoints = append(cleanup.endpoints, bad.ID)
		t.Fatalf("CreateOutboundEndpoint accepted the invalid secret %q", invalidSecret)
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusBadRequest {
		t.Fatalf("invalid secret: want a 400 APIError, got %v", err)
	}
	if strings.Contains(err.Error(), invalidSecret) || strings.Contains(apiErr.Body, invalidSecret) {
		t.Fatalf("the invalid-secret error echoes the secret")
	}

	msg2, err := api.PublishOutboundMessage(&OutboundMessagePublishOptions{
		Consumer:       name,
		EventType:      name,
		Payload:        map[string]interface{}{"order_id": name, "step": 2},
		IdempotencyKey: name + "-publish-2",
	})
	if err != nil {
		t.Fatalf("PublishOutboundMessage (2): %v", err)
	}
	importedDelivery := bin2.waitFor(ctx, t, msg2.ID, 1)
	verifyDelivery(t, importedDelivery, msg2.ID, name, map[string]interface{}{"order_id": name, "step": 2}, imported)
	if verifyStandardSignature(secret1, importedDelivery) {
		t.Fatalf("delivery to the imported-secret endpoint also verifies with another endpoint's secret")
	}

	// --- Health and endpoints across consumers ---
	health, err := api.GetOutboundHealth()
	if err != nil {
		t.Fatalf("GetOutboundHealth: %v", err)
	}
	for _, state := range []string{OutboundEndpointStateActive, OutboundEndpointStateFailing, OutboundEndpointStatePaused, OutboundEndpointStateDisabled} {
		if _, ok := health.Endpoints[state]; !ok {
			t.Fatalf("health is missing state %q: %v", state, health.Endpoints)
		}
	}
	if health.Endpoints[OutboundEndpointStateActive] < 2 {
		t.Fatalf("health counts %d active endpoints, want at least 2", health.Endpoints[OutboundEndpointStateActive])
	}
	if age := time.Since(health.Since); age < 23*time.Hour || age > 25*time.Hour {
		t.Fatalf("health window starts %s ago, want about 24h", age)
	}
	eventually(ctx, t, 30*time.Second, "health counts delivery attempts", func() bool {
		h, err := api.GetOutboundHealth()
		if err != nil {
			t.Fatalf("GetOutboundHealth: %v", err)
		}
		return h.Stats.Attempts >= 1
	})

	ours, err := api.ListAllOutboundEndpoints(&OutboundEndpointListOptions{Consumer: name, State: OutboundEndpointStateActive})
	if err != nil {
		t.Fatalf("ListAllOutboundEndpoints(consumer): %v", err)
	}
	if findEndpoint(ours, ep1.ID) == nil || findEndpoint(ours, ep2.ID) == nil {
		t.Fatalf("ListAllOutboundEndpoints(consumer=%s) returned %d endpoints, missing ours", name, len(ours))
	}
	for _, e := range ours {
		if e.Secret != "" {
			t.Fatalf("ListAllOutboundEndpoints leaks the secret of endpoint %s", e.ID)
		}
		if e.Consumer != name || e.State != OutboundEndpointStateActive {
			t.Fatalf("ListAllOutboundEndpoints filter returned %s", describeEndpoint(e))
		}
	}
	failing, err := api.ListAllOutboundEndpoints(&OutboundEndpointListOptions{State: OutboundEndpointStateFailing, Limit: 100})
	if err != nil {
		t.Fatalf("ListAllOutboundEndpoints(failing): %v", err)
	}
	for _, e := range failing {
		if e.State != OutboundEndpointStateFailing {
			t.Fatalf("state=failing filter returned %s", describeEndpoint(e))
		}
		if e.ID == ep1.ID || e.ID == ep2.ID {
			t.Fatalf("healthy endpoint %s listed as failing", e.ID)
		}
	}

	// --- Retry a delivery ---
	task, err := api.RetryOutboundDelivery(ep1.ID, msg1.ID)
	if err != nil {
		t.Fatalf("RetryOutboundDelivery: %v", err)
	}
	if task.ID == "" || task.EndpointID != ep1.ID || task.MessageID != msg1.ID || task.Kind != "retry" {
		t.Fatalf("unexpected recovery task: %+v", task)
	}
	eventually(ctx, t, 45*time.Second, "retry task completes", func() bool {
		got, err := api.GetOutboundRecoveryTask(task.ID)
		if err != nil {
			t.Fatalf("GetOutboundRecoveryTask: %v", err)
		}
		if got.Status == "failed" {
			t.Fatalf("retry task failed: %s", got.Error)
		}
		return got.Status == "completed"
	})
	retried := bin1.waitFor(ctx, t, msg1.ID, 2)
	verifyDelivery(t, retried, msg1.ID, name, payload, secret1)

	// --- Rotate the secret ---
	rotated, err := api.RotateOutboundEndpointSecret(ep1.ID)
	if err != nil {
		t.Fatalf("RotateOutboundEndpointSecret: %v", err)
	}
	if !strings.HasPrefix(rotated, "whsec_") || rotated == secret1 {
		t.Fatalf("rotation must return a new whsec_ secret (prefix ok: %t, changed: %t)", strings.HasPrefix(rotated, "whsec_"), rotated != secret1)
	}
	if revealed, err := api.RevealOutboundEndpointSecret(ep1.ID); err != nil || revealed != rotated {
		t.Fatalf("RevealOutboundEndpointSecret after rotation: matches rotated=%t, err=%v", revealed == rotated, err)
	}
	afterRotation, err := api.GetOutboundEndpoint(ep1.ID)
	if err != nil {
		t.Fatalf("GetOutboundEndpoint after rotation: %v", err)
	}
	if afterRotation.PreviousSecretExpiresAt == nil || time.Until(*afterRotation.PreviousSecretExpiresAt) < 23*time.Hour {
		t.Fatalf("previous secret must keep signing for about 24h, expires at %v", afterRotation.PreviousSecretExpiresAt)
	}
	// During the overlap every delivery carries a signature for each secret.
	msg3, err := api.PublishOutboundMessage(&OutboundMessagePublishOptions{
		Consumer:  name,
		EventType: name,
		Payload:   map[string]interface{}{"order_id": name, "step": 3},
	})
	if err != nil {
		t.Fatalf("PublishOutboundMessage (3): %v", err)
	}
	overlap := bin1.waitFor(ctx, t, msg3.ID, 1)
	verifyDelivery(t, overlap, msg3.ID, name, map[string]interface{}{"order_id": name, "step": 3}, rotated)
	if !verifyStandardSignature(secret1, overlap) {
		t.Fatalf("delivery during the rotation overlap does not verify with the previous secret")
	}
}

// outboundCleanup deletes what the test created: endpoints, then the
// consumer, then the event type (refused while an endpoint subscribes to it).
type outboundCleanup struct {
	client    *API
	endpoints []string
	consumer  string
	eventType string
}

func (c *outboundCleanup) run(t *testing.T) {
	ignoreNotFound := func(what string, err error) {
		var apiErr *APIError
		if err != nil && !(errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound) {
			t.Errorf("cleanup %s: %v", what, err)
		}
	}
	for _, id := range c.endpoints {
		ignoreNotFound("DeleteOutboundEndpoint("+id+")", c.client.DeleteOutboundEndpoint(id))
	}
	if c.consumer != "" {
		ignoreNotFound("DeleteOutboundConsumer("+c.consumer+")", c.client.DeleteOutboundConsumer(c.consumer))
	}
	if c.eventType != "" {
		ignoreNotFound("DeleteOutboundEventType("+c.eventType+")", c.client.DeleteOutboundEventType(c.eventType))
	}
}

// webhookBin talks to the free, no-auth Webhook Bin API
// (https://webhookrelay.com/webhook-bin.md).
type webhookBin struct {
	http *http.Client
}

const webhookBinAPI = "https://bin.webhookrelay.com"

type bin struct {
	client *webhookBin
	id     string
}

type binRequest struct {
	ID         string                    `json:"id"`
	ReceivedAt int64                     `json:"receivedAt"`
	Method     string                    `json:"method"`
	Header     map[string]binHeaderValue `json:"header"`
	Body       string                    `json:"body"`
}

type binHeaderValue struct {
	Key    string   `json:"key"`
	Values []string `json:"values"`
}

// get returns the first value of a header, matching its name case-insensitively.
func (r *binRequest) get(name string) string {
	for key, value := range r.Header {
		if strings.EqualFold(key, name) || strings.EqualFold(value.Key, name) {
			if len(value.Values) > 0 {
				return value.Values[0]
			}
		}
	}
	return ""
}

func (b *webhookBin) create(ctx context.Context, t *testing.T) *bin {
	t.Helper()
	var created struct {
		ID string `json:"id"`
	}
	b.do(ctx, t, http.MethodPost, webhookBinAPI+"/v1/bins", &created)
	if created.ID == "" {
		t.Fatalf("webhook bin: create returned no id")
	}
	return &bin{client: b, id: created.ID}
}

func (b *webhookBin) do(ctx context.Context, t *testing.T, method, url string, out interface{}) {
	t.Helper()
	req, err := http.NewRequestWithContext(ctx, method, url, nil)
	if err != nil {
		t.Fatalf("webhook bin: %v", err)
	}
	resp, err := b.http.Do(req)
	if err != nil {
		t.Fatalf("webhook bin %s %s: %v", method, url, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("webhook bin %s %s: %v", method, url, err)
	}
	if resp.StatusCode >= 300 {
		t.Fatalf("webhook bin %s %s: HTTP %d: %s", method, url, resp.StatusCode, body)
	}
	if err := json.Unmarshal(body, out); err != nil {
		t.Fatalf("webhook bin %s %s: decode: %v", method, url, err)
	}
}

func (b *bin) url() string {
	return webhookBinAPI + "/v1/webhooks/" + b.id
}

// requestsFor returns the captured requests whose webhook-id is messageID,
// oldest first.
func (b *bin) requestsFor(ctx context.Context, t *testing.T, messageID string) []*binRequest {
	t.Helper()
	var captured struct {
		Requests []*binRequest `json:"requests"`
	}
	b.client.do(ctx, t, http.MethodGet, webhookBinAPI+"/v1/bins/"+b.id, &captured)
	var matching []*binRequest
	for _, r := range captured.Requests {
		if r.get("webhook-id") == messageID {
			matching = append(matching, r)
		}
	}
	for i := 1; i < len(matching); i++ {
		for j := i; j > 0 && matching[j].ReceivedAt < matching[j-1].ReceivedAt; j-- {
			matching[j], matching[j-1] = matching[j-1], matching[j]
		}
	}
	return matching
}

// waitFor polls the bin for up to 60 seconds until it has captured n
// deliveries of messageID, and returns the newest.
func (b *bin) waitFor(ctx context.Context, t *testing.T, messageID string, n int) *binRequest {
	t.Helper()
	var got []*binRequest
	eventually(ctx, t, 60*time.Second, fmt.Sprintf("%d deliveries of message %s at the bin", n, messageID), func() bool {
		got = b.requestsFor(ctx, t, messageID)
		return len(got) >= n
	})
	return got[len(got)-1]
}

// verifyDelivery checks a captured delivery's Standard Webhooks headers, its
// signature with secret and that its body is the published payload.
func verifyDelivery(t *testing.T, r *binRequest, messageID, name string, payload interface{}, secret string) {
	t.Helper()
	if r.Method != http.MethodPost {
		t.Fatalf("delivery method = %s, want POST", r.Method)
	}
	id, timestamp, signature := r.get("webhook-id"), r.get("webhook-timestamp"), r.get("webhook-signature")
	if id != messageID || timestamp == "" || signature == "" {
		t.Fatalf("delivery is missing Standard Webhooks headers: webhook-id=%q webhook-timestamp=%q webhook-signature set=%t", id, timestamp, signature != "")
	}
	ts, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		t.Fatalf("webhook-timestamp %q is not unix seconds", timestamp)
	}
	if skew := time.Since(time.Unix(ts, 0)); skew < -5*time.Minute || skew > 5*time.Minute {
		t.Fatalf("webhook-timestamp is %s away from now", skew)
	}
	if !verifyStandardSignature(secret, r) {
		t.Fatalf("webhook-signature of message %s does not verify (%d signatures in header)", messageID, len(strings.Fields(signature)))
	}
	if got := r.get("webhook-event-type"); got != name {
		t.Fatalf("webhook-event-type = %q, want %q", got, name)
	}
	if got := r.get("webhook-consumer"); got != name {
		t.Fatalf("webhook-consumer = %q, want %q", got, name)
	}
	if !jsonBytesEqual(t, []byte(r.Body), payload) {
		t.Fatalf("delivery body %s does not equal the published payload", r.Body)
	}
}

// verifyStandardSignature reports whether any v1 signature in the request's
// webhook-signature header is HMAC-SHA256(key, id.timestamp.body), keyed with
// the base64-decoded secret without its whsec_ prefix.
func verifyStandardSignature(secret string, r *binRequest) bool {
	key, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(secret, "whsec_"))
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(r.get("webhook-id") + "." + r.get("webhook-timestamp") + "." + r.Body))
	expected := mac.Sum(nil)
	for _, candidate := range strings.Fields(r.get("webhook-signature")) {
		version, encoded, ok := strings.Cut(candidate, ",")
		if !ok || version != "v1" {
			continue
		}
		decoded, err := base64.StdEncoding.DecodeString(encoded)
		if err == nil && hmac.Equal(decoded, expected) {
			return true
		}
	}
	return false
}

// waitForSent polls GetOutboundMessage until the endpoint's delivery is sent.
func waitForSent(ctx context.Context, t *testing.T, api *API, messageID, endpointID string) *Log {
	t.Helper()
	var delivery *Log
	var last RequestStatus
	eventually(ctx, t, 60*time.Second, "delivery of message "+messageID+" is sent", func() bool {
		message, err := api.GetOutboundMessage(messageID)
		if err != nil {
			t.Fatalf("GetOutboundMessage: %v", err)
		}
		for _, d := range message.Deliveries {
			if d.OutputID == endpointID || len(message.Deliveries) == 1 {
				delivery, last = d, d.Status
			}
		}
		if last == RequestStatusFailed {
			t.Fatalf("delivery of message %s failed", messageID)
		}
		return delivery != nil && last == RequestStatusSent
	})
	return delivery
}

// eventually calls check every two seconds until it returns true, failing the
// test after timeout or when ctx ends.
func eventually(ctx context.Context, t *testing.T, timeout time.Duration, what string, check func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		if check() {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out after %s waiting for %s", timeout, what)
		}
		select {
		case <-ctx.Done():
			t.Fatalf("test deadline reached waiting for %s", what)
		case <-time.After(2 * time.Second):
		}
	}
}

func jsonBytesEqual(t *testing.T, raw []byte, want interface{}) bool {
	t.Helper()
	var got, expected interface{}
	if err := json.Unmarshal(raw, &got); err != nil {
		return false
	}
	wantRaw, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := json.Unmarshal(wantRaw, &expected); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return reflect.DeepEqual(got, expected)
}

// describeEndpoint formats an endpoint for failure messages without its
// signing secret.
func describeEndpoint(e *OutboundEndpoint) string {
	if e == nil {
		return "<nil>"
	}
	return fmt.Sprintf("{id=%s consumer=%s url=%s state=%s event_types=%v}", e.ID, e.Consumer, e.URL, e.State, e.EventTypes)
}

func findEndpoint(endpoints []*OutboundEndpoint, id string) *OutboundEndpoint {
	for _, e := range endpoints {
		if e.ID == id {
			return e
		}
	}
	return nil
}

func containsMessage(messages []*OutboundMessage, id string) bool {
	for _, m := range messages {
		if m.ID == id {
			return true
		}
	}
	return false
}

func containsDelivery(logs []*Log, messageID string) bool {
	for _, l := range logs {
		if l.MessageID == messageID {
			return true
		}
	}
	return false
}

func containsString(values []string, want string) bool {
	for _, v := range values {
		if v == want {
			return true
		}
	}
	return false
}

func redact(s, secret string) string {
	return strings.ReplaceAll(s, secret, "[redacted]")
}

func randomHex(t *testing.T, n int) string {
	t.Helper()
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		t.Fatalf("rand: %v", err)
	}
	return hex.EncodeToString(b)
}
