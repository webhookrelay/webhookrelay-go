package webhookrelay

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/pkg/errors"
)

// ServiceConnectionInputStatus is the health status of a service-connection input.
type ServiceConnectionInputStatus string

// Available service-connection input statuses.
const (
	ServiceConnectionInputStatusOK    ServiceConnectionInputStatus = "ok"
	ServiceConnectionInputStatusError ServiceConnectionInputStatus = "error"
)

// ServiceConnectionInputType selects the kind of source an input pulls from.
type ServiceConnectionInputType string

// Available service-connection input types.
const (
	ServiceConnectionInputTypeGCPPubSub ServiceConnectionInputType = "gcp_pubsub"
	ServiceConnectionInputTypeGCPGCS    ServiceConnectionInputType = "gcp_gcs"
	ServiceConnectionInputTypeAWSS3     ServiceConnectionInputType = "aws_s3"
	ServiceConnectionInputTypeAWSSQS    ServiceConnectionInputType = "aws_sqs"
	ServiceConnectionInputTypeAWSSNS    ServiceConnectionInputType = "aws_sns"
	ServiceConnectionInputTypeEmail     ServiceConnectionInputType = "email"
)

// ServiceConnectionInput attaches an external event source to a bucket. Events
// pulled from the source are pushed through the bucket's input -> transform ->
// outputs pipeline, exactly like an incoming webhook.
//
// Cloud types (gcp_pubsub, gcp_gcs, aws_s3, aws_sqs, aws_sns) reference a
// ServiceConnection via ServiceConnectionID for credentials. The "email" type
// is standalone (inbound email-to-webhook over Cloudflare Email Routing) and
// requires no service connection; its inbound address is surfaced in
// EmailAddress.
type ServiceConnectionInput struct {
	ID        string    `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	Name string `json:"name"`

	// ServiceConnectionID references the backing ServiceConnection (required
	// for cloud types, empty for "email").
	ServiceConnectionID string `json:"service_connection_id"`

	BucketID   string `json:"bucket_id"`
	FunctionID string `json:"function_id"`

	ServiceConnectionInputType ServiceConnectionInputType `json:"service_connection_input_type"`

	Status ServiceConnectionInputStatus `json:"status"` // readonly
	Error  string                       `json:"error"`  // readonly

	GCPPubSubInput GCPPubSubInput `json:"gcp_pubsub_input"`
	AWSS3Input     AWSS3Input     `json:"aws_s3_input"`
	AWSSQSInput    AWSSQSInput    `json:"aws_sqs_input"`
	AWSSNSInput    AWSSNSInput    `json:"aws_sns_input"`
	GCPGCSInput    GCPGCSInput    `json:"gcp_gcs_input"`
	EmailInput     EmailInput     `json:"email_input"`

	// EmailAddress is the computed inbound address for "email" inputs
	// ({input-id}@<inbound-domain>). Read-only, set by the server.
	EmailAddress string `json:"email_address,omitempty"`
}

// UnmarshalJSON accepts either unix seconds or RFC3339 strings for the time
// fields so responses decode regardless of the format the API uses.
func (i *ServiceConnectionInput) UnmarshalJSON(data []byte) error {
	type Alias ServiceConnectionInput
	aux := &struct {
		CreatedAt json.RawMessage `json:"created_at"`
		UpdatedAt json.RawMessage `json:"updated_at"`
		*Alias
	}{
		Alias: (*Alias)(i),
	}
	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}
	var err error
	if i.CreatedAt, err = parseTime(aux.CreatedAt); err != nil {
		return err
	}
	if i.UpdatedAt, err = parseTime(aux.UpdatedAt); err != nil {
		return err
	}
	return nil
}

// GCPPubSubInput configures a Google Cloud Pub/Sub subscription source.
type GCPPubSubInput struct {
	SubscriptionName string `json:"subscription_name"`
}

// GCPGCSInput configures a Google Cloud Storage object-notification source.
type GCPGCSInput struct {
	BucketName     string                  `json:"bucket_name"`
	Prefix         string                  `json:"prefix"`
	NotificationID string                  `json:"notification_id"`
	FileFormat     ObjectStorageFileFormat `json:"file_format"`
}

// AWSS3Input configures an AWS S3 object source.
type AWSS3Input struct {
	BucketName string                  `json:"bucket_name"`
	Region     string                  `json:"region"`
	Prefix     string                  `json:"prefix"`
	FileFormat ObjectStorageFileFormat `json:"file_format"`
}

// AWSSQSInput configures an AWS SQS queue source. QueueURL may be supplied as an
// SQS ARN (it is converted to a URL server-side); Region is derived from the URL
// when omitted.
type AWSSQSInput struct {
	QueueURL string `json:"queue_url"`
	Region   string `json:"region"`
}

// AWSSNSInput configures an AWS SNS topic source. Region is derived from the
// topic ARN when omitted.
type AWSSNSInput struct {
	TopicARN        string `json:"topic_arn"`
	Region          string `json:"region"`
	SubscriptionARN string `json:"subscription_arn"`
}

// EmailInput is the configuration for an "email" service-connection input
// (inbound email-to-webhook). Mail sent to the input's inbound address is parsed
// into a JSON payload and relayed through the bucket's pipeline.
type EmailInput struct {
	// Enabled gates whether inbound mail is accepted. When false the address
	// still resolves but mail is dropped at ingest.
	Enabled bool `json:"enabled"`
	// AllowedSenders, when non-empty, restricts inbound mail to these From
	// addresses (exact match, lower-cased). Max 20 entries.
	AllowedSenders []string `json:"allowed_senders,omitempty"`
	// DropAttachments skips attachment parsing and storage entirely.
	DropAttachments bool `json:"drop_attachments,omitempty"`
	// MaxAttachmentBytes overrides the per-message total attachment cap.
	// 0 uses the server default.
	MaxAttachmentBytes int64 `json:"max_attachment_bytes,omitempty"`
}

// ListServiceConnectionInputs lists service-connection inputs for a bucket
// (reference is a bucket ID or name).
func (api *API) ListServiceConnectionInputs(bucket string) ([]*ServiceConnectionInput, error) {
	bucketID, err := api.ensureBucketID(bucket)
	if err != nil {
		return nil, err
	}

	resp, err := api.makeRequest(http.MethodGet, "/buckets/"+bucketID+"/service-connection-inputs", nil)
	if err != nil {
		return nil, errors.Wrap(err, errMakeRequestError)
	}

	var inputs []*ServiceConnectionInput
	if err := json.Unmarshal(resp, &inputs); err != nil {
		return nil, errors.Wrap(err, errUnmarshalError)
	}

	return inputs, nil
}

// CreateServiceConnectionInput creates a service-connection input on the bucket
// identified by options.BucketID (ID or name).
func (api *API) CreateServiceConnectionInput(options *ServiceConnectionInput) (*ServiceConnectionInput, error) {
	bucketID, err := api.ensureBucketID(options.BucketID)
	if err != nil {
		return nil, err
	}

	resp, err := api.makeRequest(http.MethodPost, "/buckets/"+bucketID+"/service-connection-inputs", options)
	if err != nil {
		return nil, err
	}

	var input ServiceConnectionInput
	if err := json.Unmarshal(resp, &input); err != nil {
		return nil, err
	}
	return &input, nil
}

// UpdateServiceConnectionInput updates an existing service-connection input.
// options.BucketID and options.ID must be set.
func (api *API) UpdateServiceConnectionInput(options *ServiceConnectionInput) (*ServiceConnectionInput, error) {
	if options.ID == "" {
		return nil, fmt.Errorf("service connection input ID must be supplied")
	}

	bucketID, err := api.ensureBucketID(options.BucketID)
	if err != nil {
		return nil, err
	}

	resp, err := api.makeRequest(http.MethodPut, "/buckets/"+bucketID+"/service-connection-inputs/"+options.ID, options)
	if err != nil {
		return nil, err
	}

	var input ServiceConnectionInput
	if err := json.Unmarshal(resp, &input); err != nil {
		return nil, err
	}
	return &input, nil
}

// DeleteServiceConnectionInput deletes a service-connection input from a bucket.
func (api *API) DeleteServiceConnectionInput(bucket, id string) error {
	if id == "" {
		return fmt.Errorf("service connection input ID must be supplied")
	}

	bucketID, err := api.ensureBucketID(bucket)
	if err != nil {
		return err
	}

	_, err = api.makeRequest(http.MethodDelete, "/buckets/"+bucketID+"/service-connection-inputs/"+id, nil)
	return err
}
