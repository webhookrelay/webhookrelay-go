package webhookrelay

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/pkg/errors"
)

// ServiceConnectionOutputType selects the kind of destination an output pushes to.
type ServiceConnectionOutputType string

// Available service-connection output types.
const (
	ServiceConnectionOutputTypeGCPPubSub ServiceConnectionOutputType = "gcp_pubsub"
	ServiceConnectionOutputTypeGCPGCS    ServiceConnectionOutputType = "gcp_gcs"
	ServiceConnectionOutputTypeAWSS3     ServiceConnectionOutputType = "aws_s3"
	ServiceConnectionOutputTypeAWSSQS    ServiceConnectionOutputType = "aws_sqs"
	ServiceConnectionOutputTypeAWSSNS    ServiceConnectionOutputType = "aws_sns"
	ServiceConnectionOutputTypeDiscord   ServiceConnectionOutputType = "discord"
	ServiceConnectionOutputTypeSlack     ServiceConnectionOutputType = "slack"
)

// ObjectStorageFileFormat controls how webhooks are serialized when written to
// object storage (S3, GCS) sources/destinations.
type ObjectStorageFileFormat string

// Available object-storage file formats.
const (
	ObjectStorageFileFormatJSON ObjectStorageFileFormat = "json"      // headers, body, etc.
	ObjectStorageFileFormatBody ObjectStorageFileFormat = "body_only" // raw body only
	ObjectStorageFileFormatHAR  ObjectStorageFileFormat = "har"       // HTTP Archive format
)

// ServiceConnectionOutput pushes a bucket's webhooks to an external destination.
// Cloud types (gcp_pubsub, gcp_gcs, aws_s3, aws_sqs, aws_sns) reference a
// ServiceConnection via ServiceConnectionID. The discord and slack types are
// standalone (auth is embedded in the webhook URL) and need no service
// connection — those destinations are live-tested on create and update.
type ServiceConnectionOutput struct {
	ID        string    `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	Name string `json:"name"`

	// ServiceConnectionID references the backing ServiceConnection (required for
	// cloud types, empty for discord/slack).
	ServiceConnectionID string `json:"service_connection_id"`

	BucketID   string `json:"bucket_id"`
	FunctionID string `json:"function_id"`

	ServiceConnectionOutputType ServiceConnectionOutputType `json:"service_connection_output_type"`

	GCPPubSubOutput *GCPPubSubOutput `json:"gcp_pubsub_output,omitempty"`
	AWSS3Output     *AWSS3Output     `json:"aws_s3_output,omitempty"`
	AWSSQSOutput    *AWSSQSOutput    `json:"aws_sqs_output,omitempty"`
	AWSSNSOutput    *AWSSNSOutput    `json:"aws_sns_output,omitempty"`
	GCPGCSOutput    *GCPGCSOutput    `json:"gcp_gcs_output,omitempty"`
	DiscordOutput   *DiscordOutput   `json:"discord_output,omitempty"`
	SlackOutput     *SlackOutput     `json:"slack_output,omitempty"`
}

// GCPPubSubOutput configures a Google Cloud Pub/Sub topic destination.
type GCPPubSubOutput struct {
	TopicName string `json:"topic_name"`
}

// GCPGCSOutput configures a Google Cloud Storage destination.
type GCPGCSOutput struct {
	BucketName string                  `json:"bucket_name"`
	Prefix     string                  `json:"prefix"`
	FileFormat ObjectStorageFileFormat `json:"file_format"`
}

// AWSS3Output configures an AWS S3 destination.
type AWSS3Output struct {
	BucketName string                  `json:"bucket_name"`
	Region     string                  `json:"region"`
	Prefix     string                  `json:"prefix"`
	FileFormat ObjectStorageFileFormat `json:"file_format"`
}

// AWSSQSOutput configures an AWS SQS queue destination. QueueURL may be an ARN
// (converted server-side); Region is derived from the URL when omitted.
type AWSSQSOutput struct {
	QueueURL string `json:"queue_url"`
	Region   string `json:"region"`
}

// AWSSNSOutput configures an AWS SNS topic destination. Region is derived from
// the topic ARN when omitted.
type AWSSNSOutput struct {
	TopicARN string `json:"topic_arn"`
	Region   string `json:"region"`
}

// DiscordOutput configures a Discord incoming-webhook destination.
type DiscordOutput struct {
	WebhookURL string `json:"webhook_url"`
}

// SlackOutput configures a Slack incoming-webhook destination.
type SlackOutput struct {
	WebhookURL string `json:"webhook_url"`
}

// ListServiceConnectionOutputs lists service-connection outputs for a bucket
// (reference is a bucket ID or name).
func (api *API) ListServiceConnectionOutputs(bucket string) ([]*ServiceConnectionOutput, error) {
	bucketID, err := api.ensureBucketID(bucket)
	if err != nil {
		return nil, err
	}

	resp, err := api.makeRequest(http.MethodGet, "/buckets/"+bucketID+"/service-connection-outputs", nil)
	if err != nil {
		return nil, errors.Wrap(err, errMakeRequestError)
	}

	var outputs []*ServiceConnectionOutput
	if err := json.Unmarshal(resp, &outputs); err != nil {
		return nil, errors.Wrap(err, errUnmarshalError)
	}

	return outputs, nil
}

// CreateServiceConnectionOutput creates a service-connection output on the bucket
// identified by options.BucketID (ID or name).
func (api *API) CreateServiceConnectionOutput(options *ServiceConnectionOutput) (*ServiceConnectionOutput, error) {
	bucketID, err := api.ensureBucketID(options.BucketID)
	if err != nil {
		return nil, err
	}

	resp, err := api.makeRequest(http.MethodPost, "/buckets/"+bucketID+"/service-connection-outputs", options)
	if err != nil {
		return nil, err
	}

	var output ServiceConnectionOutput
	if err := json.Unmarshal(resp, &output); err != nil {
		return nil, err
	}
	return &output, nil
}

// UpdateServiceConnectionOutput updates an existing service-connection output.
// options.BucketID and options.ID must be set.
func (api *API) UpdateServiceConnectionOutput(options *ServiceConnectionOutput) (*ServiceConnectionOutput, error) {
	if options.ID == "" {
		return nil, fmt.Errorf("service connection output ID must be supplied")
	}

	bucketID, err := api.ensureBucketID(options.BucketID)
	if err != nil {
		return nil, err
	}

	resp, err := api.makeRequest(http.MethodPut, "/buckets/"+bucketID+"/service-connection-outputs/"+options.ID, options)
	if err != nil {
		return nil, err
	}

	var output ServiceConnectionOutput
	if err := json.Unmarshal(resp, &output); err != nil {
		return nil, err
	}
	return &output, nil
}

// DeleteServiceConnectionOutput deletes a service-connection output from a bucket.
func (api *API) DeleteServiceConnectionOutput(bucket, id string) error {
	if id == "" {
		return fmt.Errorf("service connection output ID must be supplied")
	}

	bucketID, err := api.ensureBucketID(bucket)
	if err != nil {
		return err
	}

	_, err = api.makeRequest(http.MethodDelete, "/buckets/"+bucketID+"/service-connection-outputs/"+id, nil)
	return err
}
