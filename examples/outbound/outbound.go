// Package outbound is a runnable example of sending outbound webhooks to your
// customers. outbound_test.go runs it, and the Webhook Relay dashboard shows
// the marked region, so keep the two in step.
package outbound

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/webhookrelay/webhookrelay-go"
)

// dashboard-snippet:start
// NewClient reads your API key from the environment. Keep it server-side.
func NewClient() (*webhookrelay.API, error) {
	return webhookrelay.NewWithAPIKey(os.Getenv("RELAY_API_KEY"))
}

// OnboardCustomer runs once per customer: it registers the HTTPS endpoint they give you.
func OnboardCustomer(api *webhookrelay.API, customerURL string) (*webhookrelay.OutboundEndpoint, error) {
	_, err := api.UpsertOutboundEventType(&webhookrelay.OutboundEventTypeOptions{
		Name:        "invoice.paid",
		Description: "An invoice was paid",
	})
	if err != nil {
		return nil, err
	}
	_, err = api.UpsertOutboundConsumer(&webhookrelay.OutboundConsumerOptions{ID: "customer_42", Name: "Acme"})
	if err != nil {
		return nil, err
	}
	return api.CreateOutboundEndpoint(&webhookrelay.OutboundEndpointOptions{
		Consumer:   "customer_42",
		URL:        customerURL,
		EventTypes: []string{"invoice.paid"},
	})
}

// PublishInvoicePaid runs every time the event happens. Retry with the same idempotency key.
func PublishInvoicePaid(api *webhookrelay.API) (*webhookrelay.OutboundMessage, error) {
	return api.PublishOutboundMessage(&webhookrelay.OutboundMessagePublishOptions{
		Consumer:       "customer_42",
		EventType:      "invoice.paid",
		Payload:        map[string]interface{}{"invoice_id": "inv_123", "amount": 4900},
		IdempotencyKey: "invoice-paid:inv_123",
	})
}

// dashboard-snippet:end

// FailingEndpoints lists the endpoints that are failing, longest failing
// first, with their failures over the last 24 hours.
func FailingEndpoints(api *webhookrelay.API) ([]*webhookrelay.OutboundEndpoint, error) {
	health, err := api.GetOutboundHealth()
	if err != nil {
		return nil, err
	}
	if health.Endpoints[webhookrelay.OutboundEndpointStateFailing] == 0 {
		return nil, nil
	}
	return api.ListAllOutboundEndpoints(&webhookrelay.OutboundEndpointListOptions{
		State: webhookrelay.OutboundEndpointStateFailing,
		Limit: 20,
	})
}

// CreateQueueEndpoint delivers a customer's events to their own SQS queue
// instead of an HTTPS URL. The access key is theirs, scoped to sending to that
// queue; it is stored encrypted and never returned (the endpoint shows its
// key ID in CredentialsIdentity).
func CreateQueueEndpoint(api *webhookrelay.API, queueURL, accessKeyID, secretAccessKey string) (*webhookrelay.OutboundEndpoint, error) {
	return api.CreateOutboundEndpoint(&webhookrelay.OutboundEndpointOptions{
		Consumer:    "customer_42",
		Type:        webhookrelay.OutboundEndpointTypeAWSSQS,
		Destination: &webhookrelay.OutboundDestination{AWSSQS: &webhookrelay.OutboundAWSSQS{QueueURL: queueURL}},
		Credentials: &webhookrelay.OutboundEndpointCredentials{AWS: &webhookrelay.OutboundAWSCredentials{
			AccessKeyID:     accessKeyID,
			SecretAccessKey: secretAccessKey,
		}},
		EventTypes: []string{"invoice.paid"},
	})
}

// ImportCustomers migrates customers from another sender in one request and
// waits for the import to finish, checking every poll until ctx is done. A
// refused import returns a 400 *webhookrelay.APIError whose ImportErrors list
// the items to fix (nothing was written then); a failed one returns the task,
// whose Error and Errors say why, with an error.
func ImportCustomers(ctx context.Context, api *webhookrelay.API, customerURL string, poll time.Duration) (*webhookrelay.OutboundImportTask, error) {
	api = api.WithContext(ctx)
	task, err := api.StartOutboundImport(&webhookrelay.OutboundImportOptions{
		EventTypes: []webhookrelay.OutboundImportEventType{{Name: "invoice.paid"}},
		Consumers:  []webhookrelay.OutboundImportConsumer{{ID: "customer_42", Name: "Acme"}},
		Endpoints: []webhookrelay.OutboundImportEndpoint{{
			Consumer:   "customer_42",
			URL:        customerURL,
			EventTypes: []string{"invoice.paid"},
			Paused:     true, // resume once you stop sending from the old system
		}},
	})
	if err != nil {
		return nil, err
	}
	for !task.Done() {
		select {
		case <-ctx.Done():
			return task, fmt.Errorf("import %s still %s: %w", task.ID, task.Status, ctx.Err())
		case <-time.After(poll):
		}
		if task, err = api.GetOutboundImport(task.ID); err != nil {
			return nil, err
		}
	}
	if task.Status != webhookrelay.OutboundImportStatusCompleted {
		return task, fmt.Errorf("import %s %s: %s", task.ID, task.Status, task.Error)
	}
	return task, nil
}

// CreateRegionalEndpoint subscribes a customer's endpoint to one channel and
// filters on the payload: it only receives invoice.paid messages published to
// the "eu" channel for invoices of at least 100 that were not refunded.
func CreateRegionalEndpoint(api *webhookrelay.API, customerURL string) (*webhookrelay.OutboundEndpoint, error) {
	return api.CreateOutboundEndpoint(&webhookrelay.OutboundEndpointOptions{
		Consumer:   "customer_42",
		URL:        customerURL,
		EventTypes: []string{"invoice.paid"},
		Channels:   []string{"eu"},
		Filter:     webhookrelay.OutboundFilter(`{"amount": {"$gte": 100}, "refund": {"$exists": false}}`),
	})
}

// PublishToRegion publishes to the "eu" channel: endpoints with other
// channels do not receive it, endpoints without channels do not either.
func PublishToRegion(api *webhookrelay.API) (*webhookrelay.OutboundMessage, error) {
	return api.PublishOutboundMessage(&webhookrelay.OutboundMessagePublishOptions{
		Consumer:       "customer_42",
		EventType:      "invoice.paid",
		Payload:        map[string]interface{}{"invoice_id": "inv_124", "amount": 4900},
		Channels:       []string{"eu"},
		IdempotencyKey: "invoice-paid:inv_124",
	})
}

// MoveEndpointToRegion changes one setting of an endpoint. Updates replace
// the whole configuration, so it starts from the endpoint's current one.
func MoveEndpointToRegion(api *webhookrelay.API, endpointID, channel string) (*webhookrelay.OutboundEndpoint, error) {
	endpoint, err := api.GetOutboundEndpoint(endpointID)
	if err != nil {
		return nil, err
	}
	options := endpoint.UpdateOptions()
	options.Channels = []string{channel}
	return api.UpdateOutboundEndpoint(endpointID, options)
}

// SendTestEvent sends the event type's example payload to one endpoint, for
// example from a "send test" button. The endpoint's channels and filter do
// not apply: the test reaches the endpoint whatever its payload.
func SendTestEvent(api *webhookrelay.API, endpointID string) (*webhookrelay.OutboundMessage, error) {
	return api.SendOutboundTest(endpointID, &webhookrelay.OutboundTestOptions{EventType: "invoice.paid"})
}

// SetUpTestEnvironment creates a "test" environment, once, and returns a client
// acting in it; the original client keeps acting in the default environment.
// Clients can also start in one: webhookrelay.WithOutboundEnvironment("test").
func SetUpTestEnvironment(api *webhookrelay.API) (*webhookrelay.API, error) {
	_, err := api.CreateOutboundEnvironment(&webhookrelay.OutboundEnvironmentOptions{Name: "test", Description: "Staging"})
	if err != nil && !webhookrelay.IsConflict(err) { // 409: it already exists
		return nil, err
	}
	return api.OutboundEnvironment("test"), nil
}

// PortalURL mints a consumer portal link for one page load of your app, tagged
// with your user's session so SignOut can end it. Never log the URL: it
// carries a one-time token.
func PortalURL(api *webhookrelay.API, userSession string) (string, error) {
	link, err := api.CreateOutboundPortalSession("customer_42", &webhookrelay.OutboundPortalSessionOptions{
		TTL:       time.Hour,
		SessionID: userSession,
	})
	if err != nil {
		return "", err
	}
	return link.URL, nil
}

// SignOut ends the portal sessions opened for your user's session.
func SignOut(api *webhookrelay.API, userSession string) (int64, error) {
	return api.ExpireOutboundPortalSessions("customer_42", userSession)
}
