// Package outbound is a runnable example of sending outbound webhooks to your
// customers. outbound_test.go runs it, and the Webhook Relay dashboard shows
// the marked region, so keep the two in step.
package outbound

import (
	"os"

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
