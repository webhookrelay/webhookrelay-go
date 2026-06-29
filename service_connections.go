package webhookrelay

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/pkg/errors"
)

// ServiceConnectionStatus is the connection health status.
type ServiceConnectionStatus string

// Available service connection statuses.
const (
	ServiceConnectionStatusPending   ServiceConnectionStatus = "pending"
	ServiceConnectionStatusConnected ServiceConnectionStatus = "connected"
	ServiceConnectionStatusError     ServiceConnectionStatus = "error"
)

// ServiceType identifies the cloud provider a service connection targets.
type ServiceType string

// Available service connection provider types.
const (
	ServiceGCP   ServiceType = "gcp"
	ServiceAWS   ServiceType = "aws"
	ServiceAzure ServiceType = "azure"
)

// ServiceConnection stores credentials for a 3rd party cloud provider. It is
// account-scoped and referenced by service-connection inputs/outputs to either
// subscribe to events (SNS, GCP Pub/Sub, ...) or push webhooks into a cloud
// service (S3, GCS, Pub/Sub, ...).
//
// Secret fields (service account keys, secret access keys, client secrets) are
// write-only: they are accepted on create/update but redacted ("****") in API
// responses. To keep an existing secret on update, leave the secret field empty.
type ServiceConnection struct {
	ID          string                  `json:"id"` // ULID, prefixed with "scn_"
	Name        string                  `json:"name"`
	CreatedAt   time.Time               `json:"created_at"`
	UpdatedAt   time.Time               `json:"updated_at"`
	Status      ServiceConnectionStatus `json:"status"`       // readonly
	LastChecked time.Time               `json:"last_checked"` // readonly
	Error       string                  `json:"error"`        // readonly
	Retries     int                     `json:"retries"`      // readonly
	AccountID   string                  `json:"account_id"`   // readonly

	// ServiceType selects which provider sub-struct is used.
	ServiceType ServiceType `json:"service_type"`

	GCPServiceConnection   GCPServiceConnection   `json:"gcp_service_connection"`
	AWSServiceConnection   AWSServiceConnection   `json:"aws_service_connection"`
	AzureServiceConnection AzureServiceConnection `json:"azure_service_connection"`
}

// GCPServiceConnection holds Google Cloud credentials.
type GCPServiceConnection struct {
	ProjectID           string `json:"project_id"`
	ServiceAccountEmail string `json:"service_account_email"`
	ServiceAccountKey   string `json:"service_account_key"` // write-only, redacted in responses
}

// AWSServiceConnection holds AWS credentials.
type AWSServiceConnection struct {
	AccessKeyID     string `json:"access_key_id"`
	SecretAccessKey string `json:"secret_access_key"` // write-only, redacted in responses
}

// AzureServiceConnection holds Azure credentials.
type AzureServiceConnection struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"` // write-only, redacted in responses
	TenantID     string `json:"tenant_id"`
}

// ServiceConnectionListOptions is used to list service connections.
type ServiceConnectionListOptions struct{}

// ListServiceConnections lists service connections for an account.
func (api *API) ListServiceConnections(options *ServiceConnectionListOptions) ([]*ServiceConnection, error) {
	resp, err := api.makeRequest(http.MethodGet, "/service-connections", nil)
	if err != nil {
		return nil, errors.Wrap(err, errMakeRequestError)
	}

	var connections []*ServiceConnection
	if err := json.Unmarshal(resp, &connections); err != nil {
		return nil, errors.Wrap(err, errUnmarshalError)
	}

	return connections, nil
}

// GetServiceConnection gets a service connection by ID.
func (api *API) GetServiceConnection(id string) (*ServiceConnection, error) {
	resp, err := api.makeRequest(http.MethodGet, "/service-connections/"+id, nil)
	if err != nil {
		return nil, err
	}

	var connection ServiceConnection
	if err := json.Unmarshal(resp, &connection); err != nil {
		return nil, err
	}
	return &connection, nil
}

// CreateServiceConnection creates a new service connection.
func (api *API) CreateServiceConnection(options *ServiceConnection) (*ServiceConnection, error) {
	resp, err := api.makeRequest(http.MethodPost, "/service-connections", options)
	if err != nil {
		return nil, err
	}

	var connection ServiceConnection
	if err := json.Unmarshal(resp, &connection); err != nil {
		return nil, err
	}
	return &connection, nil
}

// UpdateServiceConnection updates an existing service connection. The ID must be
// set. ServiceType is immutable; leave secret fields empty to keep them.
func (api *API) UpdateServiceConnection(options *ServiceConnection) (*ServiceConnection, error) {
	if options.ID == "" {
		return nil, fmt.Errorf("service connection ID must be supplied")
	}

	resp, err := api.makeRequest(http.MethodPut, "/service-connections/"+options.ID, options)
	if err != nil {
		return nil, err
	}

	var connection ServiceConnection
	if err := json.Unmarshal(resp, &connection); err != nil {
		return nil, err
	}
	return &connection, nil
}

// DeleteServiceConnection deletes a service connection by ID.
func (api *API) DeleteServiceConnection(id string) error {
	if id == "" {
		return fmt.Errorf("service connection ID must be supplied")
	}

	_, err := api.makeRequest(http.MethodDelete, "/service-connections/"+id, nil)
	return err
}
