// Package outbound is a runnable example of sending outbound webhooks to your
// customers. outbound_test.go runs it, and the Webhook Relay dashboard shows
// the marked region, so keep the two in step.
package outbound

import (
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

// ImportCustomers migrates customers from another sender in one request and
// waits for the import to finish, checking every poll. A refused import
// returns a 400 *webhookrelay.APIError whose ImportErrors list the items to
// fix; nothing was written then. Bound a large import with api.WithContext.
func ImportCustomers(api *webhookrelay.API, customerURL string, poll time.Duration) (*webhookrelay.OutboundImport, error) {
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
		time.Sleep(poll)
		if task, err = api.GetOutboundImport(task.ID); err != nil {
			return nil, err
		}
	}
	return task, nil
}
