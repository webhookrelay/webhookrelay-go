package webhookrelay

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/pkg/errors"
)

// IntegrationPlugin identifies the notification plugin an integration uses.
type IntegrationPlugin string

// Available integration plugins.
const (
	IntegrationPluginWebhook IntegrationPlugin = "webhook"
	IntegrationPluginSlack   IntegrationPlugin = "slack"
)

// Notification event types that an integration can fire on.
const (
	NotificationEventForwardingFunctionError = "forwarding.function.error"
	NotificationEventForwardingDeliveryError = "forwarding.delivery.error"
)

// IntegrationConfiguration configures notifications/alerts for account events
// (e.g. forwarding/function errors) delivered via a plugin such as Slack or a
// generic webhook. It is linked to one or more buckets, managed through
// AddIntegrationBucket / RemoveIntegrationBucket (not the Buckets field on
// update).
//
// Note: despite the /integrations route, this is unrelated to service
// connections — it is purely alerting configuration.
type IntegrationConfiguration struct {
	ID        string    `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// Buckets are the buckets this integration is linked to. Read-only here;
	// change membership via AddIntegrationBucket / RemoveIntegrationBucket.
	Buckets []*Bucket `json:"buckets"`

	// Disabled turns the notification off without deleting it.
	Disabled bool `json:"disabled"`
	// Events is the list of event types to fire on, e.g.
	// NotificationEventForwardingFunctionError.
	Events []string `json:"events"`
	// Plugin is the notification plugin (immutable after create).
	Plugin IntegrationPlugin `json:"plugin"`
	// Configuration is free-form plugin config. Both plugins require a
	// "webhook_url" entry.
	Configuration map[string]interface{} `json:"configuration"`

	Status  string `json:"status"`  // readonly, set by the plugin
	Message string `json:"message"` // readonly, set by the plugin

	Description string `json:"description"`
}

// MarshalJSON helper to change time into unix.
func (d *IntegrationConfiguration) MarshalJSON() ([]byte, error) {
	type Alias IntegrationConfiguration
	return json.Marshal(&struct {
		CreatedAt int64 `json:"created_at"`
		UpdatedAt int64 `json:"updated_at"`
		*Alias
	}{
		CreatedAt: d.CreatedAt.Unix(),
		UpdatedAt: d.UpdatedAt.Unix(),
		Alias:     (*Alias)(d),
	})
}

// UnmarshalJSON helper to change time from unix or RFC3339 string.
func (d *IntegrationConfiguration) UnmarshalJSON(data []byte) error {
	type Alias IntegrationConfiguration
	aux := &struct {
		CreatedAt json.RawMessage `json:"created_at"`
		UpdatedAt json.RawMessage `json:"updated_at"`
		*Alias
	}{
		Alias: (*Alias)(d),
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	var err error
	d.CreatedAt, err = parseTime(aux.CreatedAt)
	if err != nil {
		return err
	}
	d.UpdatedAt, err = parseTime(aux.UpdatedAt)
	if err != nil {
		return err
	}
	return nil
}

// integrationConfigurationAddBucket is the request body for AddIntegrationBucket.
type integrationConfigurationAddBucket struct {
	BucketID string `json:"bucket_id"`
}

// IntegrationListOptions is used to list integrations.
type IntegrationListOptions struct{}

// ListIntegrations lists integration (notification) configurations for an account.
func (api *API) ListIntegrations(options *IntegrationListOptions) ([]*IntegrationConfiguration, error) {
	resp, err := api.makeRequest(http.MethodGet, "/integrations", nil)
	if err != nil {
		return nil, errors.Wrap(err, errMakeRequestError)
	}

	var integrations []*IntegrationConfiguration
	if err := json.Unmarshal(resp, &integrations); err != nil {
		return nil, errors.Wrap(err, errUnmarshalError)
	}

	return integrations, nil
}

// GetIntegration gets an integration configuration by ID.
func (api *API) GetIntegration(id string) (*IntegrationConfiguration, error) {
	resp, err := api.makeRequest(http.MethodGet, "/integrations/"+id, nil)
	if err != nil {
		return nil, err
	}

	var integration IntegrationConfiguration
	if err := json.Unmarshal(resp, &integration); err != nil {
		return nil, err
	}
	return &integration, nil
}

// CreateIntegration creates a new integration configuration. Plugin and a
// configuration["webhook_url"] are required.
func (api *API) CreateIntegration(options *IntegrationConfiguration) (*IntegrationConfiguration, error) {
	resp, err := api.makeRequest(http.MethodPost, "/integrations", options)
	if err != nil {
		return nil, err
	}

	var integration IntegrationConfiguration
	if err := json.Unmarshal(resp, &integration); err != nil {
		return nil, err
	}
	return &integration, nil
}

// UpdateIntegration updates an existing integration configuration. The ID must
// be set. Plugin and bucket membership are immutable through this call.
func (api *API) UpdateIntegration(options *IntegrationConfiguration) (*IntegrationConfiguration, error) {
	if options.ID == "" {
		return nil, fmt.Errorf("integration ID must be supplied")
	}

	resp, err := api.makeRequest(http.MethodPut, "/integrations/"+options.ID, options)
	if err != nil {
		return nil, err
	}

	var integration IntegrationConfiguration
	if err := json.Unmarshal(resp, &integration); err != nil {
		return nil, err
	}
	return &integration, nil
}

// DeleteIntegration deletes an integration configuration by ID.
func (api *API) DeleteIntegration(id string) error {
	if id == "" {
		return fmt.Errorf("integration ID must be supplied")
	}

	_, err := api.makeRequest(http.MethodDelete, "/integrations/"+id, nil)
	return err
}

// AddIntegrationBucket links a bucket (ID or name) to an integration and returns
// the updated integration configuration.
func (api *API) AddIntegrationBucket(integrationID, bucket string) (*IntegrationConfiguration, error) {
	bucketID, err := api.ensureBucketID(bucket)
	if err != nil {
		return nil, err
	}

	resp, err := api.makeRequest(http.MethodPost, "/integrations/"+integrationID+"/buckets", &integrationConfigurationAddBucket{BucketID: bucketID})
	if err != nil {
		return nil, err
	}

	var integration IntegrationConfiguration
	if err := json.Unmarshal(resp, &integration); err != nil {
		return nil, err
	}
	return &integration, nil
}

// RemoveIntegrationBucket unlinks a bucket (ID or name) from an integration and
// returns the updated integration configuration.
func (api *API) RemoveIntegrationBucket(integrationID, bucket string) (*IntegrationConfiguration, error) {
	bucketID, err := api.ensureBucketID(bucket)
	if err != nil {
		return nil, err
	}

	resp, err := api.makeRequest(http.MethodDelete, "/integrations/"+integrationID+"/buckets/"+bucketID, nil)
	if err != nil {
		return nil, err
	}

	var integration IntegrationConfiguration
	if err := json.Unmarshal(resp, &integration); err != nil {
		return nil, err
	}
	return &integration, nil
}
