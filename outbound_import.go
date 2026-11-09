package webhookrelay

import (
	"net/http"
	"time"
)

// Bulk import creates many event types, consumers and endpoints in one
// request, for example when migrating from another webhook sender. The whole
// import is validated before anything is written, then runs in the background.
// Items that already exist (event types by name, consumers by ID, endpoints by
// URL or cloud destination per consumer) are left unchanged, so re-running an
// import is safe.
const (
	// OutboundMaxImportItems caps the event types, consumers and endpoints one
	// import carries together. Split larger migrations into several imports.
	OutboundMaxImportItems = 5000
	// OutboundMaxImportBytes caps an import's request body; larger bodies are
	// refused with a 413 APIError.
	OutboundMaxImportBytes = 10 << 20
)

// Outbound endpoint types: an HTTPS webhook, or a queue, topic or bucket in
// the consumer's own cloud account.
const (
	OutboundEndpointTypeWebhook   = "webhook"
	OutboundEndpointTypeAWSSQS    = "aws_sqs"
	OutboundEndpointTypeAWSSNS    = "aws_sns"
	OutboundEndpointTypeAWSS3     = "aws_s3"
	OutboundEndpointTypeGCPPubSub = "gcp_pubsub"
	OutboundEndpointTypeGCPGCS    = "gcp_gcs"
)

// OutboundDestination configures a cloud endpoint. Set exactly the block that
// matches the endpoint's type; webhooks have none.
type OutboundDestination struct {
	AWSSQS    *OutboundAWSSQS    `json:"aws_sqs,omitempty"`
	AWSSNS    *OutboundAWSSNS    `json:"aws_sns,omitempty"`
	AWSS3     *OutboundAWSS3     `json:"aws_s3,omitempty"`
	GCPPubSub *OutboundGCPPubSub `json:"gcp_pubsub,omitempty"`
	GCPGCS    *OutboundGCPGCS    `json:"gcp_gcs,omitempty"`
}

// OutboundAWSSQS delivers to an SQS queue.
type OutboundAWSSQS struct {
	QueueURL string `json:"queue_url"`
	// Region defaults to the one in an AWS queue URL.
	Region string `json:"region,omitempty"`
	// EndpointURL points at an AWS-compatible service instead of AWS.
	EndpointURL string `json:"endpoint_url,omitempty"`
}

// OutboundAWSSNS delivers to an SNS topic.
type OutboundAWSSNS struct {
	TopicARN string `json:"topic_arn"`
	// Region defaults to the one in the topic ARN.
	Region string `json:"region,omitempty"`
	// EndpointURL points at an AWS-compatible service instead of AWS.
	EndpointURL string `json:"endpoint_url,omitempty"`
}

// OutboundAWSS3 writes each message as an object to an S3 bucket.
type OutboundAWSS3 struct {
	Bucket string `json:"bucket"`
	Region string `json:"region"`
	// Prefix is prepended to every object name.
	Prefix string `json:"prefix,omitempty"`
	// EndpointURL points at an AWS-compatible service (MinIO, Cloudflare R2)
	// instead of AWS.
	EndpointURL string `json:"endpoint_url,omitempty"`
}

// OutboundGCPPubSub delivers to a Pub/Sub topic.
type OutboundGCPPubSub struct {
	// Topic is "projects/{project}/topics/{topic}", or a topic ID in the
	// service account's project.
	Topic string `json:"topic"`
}

// OutboundGCPGCS writes each message as an object to a Cloud Storage bucket.
type OutboundGCPGCS struct {
	Bucket string `json:"bucket"`
	// Prefix is prepended to every object name.
	Prefix string `json:"prefix,omitempty"`
}

// OutboundEndpointCredentials authenticate a cloud endpoint against the
// consumer's account. They are write-only: stored encrypted and never
// returned.
type OutboundEndpointCredentials struct {
	AWS *OutboundAWSCredentials `json:"aws,omitempty"`
	GCP *OutboundGCPCredentials `json:"gcp,omitempty"`
}

// OutboundAWSCredentials is an AWS access key.
type OutboundAWSCredentials struct {
	AccessKeyID     string `json:"access_key_id"`
	SecretAccessKey string `json:"secret_access_key"`
}

// OutboundGCPCredentials is a GCP service account key.
type OutboundGCPCredentials struct {
	// ServiceAccountKey is the service account's JSON key file.
	ServiceAccountKey string `json:"service_account_key"`
}

// OutboundImportOptions is a bulk import. Event types are created first, then
// consumers, then endpoints, so endpoints may name consumers and event types
// created by the same import.
type OutboundImportOptions struct {
	EventTypes []OutboundImportEventType `json:"event_types,omitempty"`
	Consumers  []OutboundImportConsumer  `json:"consumers,omitempty"`
	Endpoints  []OutboundImportEndpoint  `json:"endpoints,omitempty"`
}

// OutboundImportEventType is an event type to create unless one with its name
// exists.
type OutboundImportEventType struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	// Example is any value that encodes to JSON.
	Example    interface{} `json:"example,omitempty"`
	Deprecated bool        `json:"deprecated,omitempty"`
}

// OutboundImportConsumer is a consumer to create unless one with its ID exists.
type OutboundImportConsumer struct {
	ID   string `json:"id"`
	Name string `json:"name,omitempty"`
	// Rate is the default deliveries per second for the consumer's endpoints;
	// 0 means 50.
	Rate int `json:"rate,omitempty"`
}

// OutboundImportEndpoint is an endpoint to create for a consumer unless the
// consumer already has one with the same URL or cloud destination. It takes
// the OutboundEndpointOptions fields plus the cloud destination and whether it
// starts paused.
type OutboundImportEndpoint struct {
	// Consumer is the endpoint's consumer ID, existing or in the same import.
	Consumer string `json:"consumer"`
	// Type is an OutboundEndpointType* constant; empty means a webhook.
	Type string `json:"type,omitempty"`
	// URL is a webhook endpoint's public HTTPS URL.
	URL string `json:"url,omitempty"`
	// Destination configures a cloud endpoint.
	Destination *OutboundDestination `json:"destination,omitempty"`
	// Credentials authenticate a cloud endpoint; write-only.
	Credentials *OutboundEndpointCredentials `json:"credentials,omitempty"`
	// Secret imports an existing Standard Webhooks signing secret (e.g. from
	// Svix) so the consumer keeps verifying with it; omit it to generate one.
	// Write-only, never returned.
	Secret string `json:"secret,omitempty"`
	// EventTypes lists the event types to deliver, or []string{"*"} for all.
	EventTypes  []string          `json:"event_types"`
	Description string            `json:"description,omitempty"`
	Headers     map[string]string `json:"headers,omitempty"`
	// Rate is deliveries per second; 0 uses the consumer's rate.
	Rate int `json:"rate,omitempty"`
	// Timeout is seconds per attempt, up to 60; 0 means 15.
	Timeout     int    `json:"timeout,omitempty"`
	AutoDisable bool   `json:"auto_disable,omitempty"`
	FunctionID  string `json:"function_id,omitempty"`
	// Paused creates the endpoint paused, for a cutover: publish to both
	// senders, then resume each consumer's endpoints. Messages published while
	// it is paused are recorded as skipped.
	Paused bool `json:"paused,omitempty"`
}

// Kinds of OutboundImportItemError; OutboundImportItemLimit is an error about
// the import as a whole.
const (
	OutboundImportItemEventType = "event_type"
	OutboundImportItemConsumer  = "consumer"
	OutboundImportItemEndpoint  = "endpoint"
	OutboundImportItemLimit     = "limit"
)

// OutboundImportItemError is why one item, or the import as a whole, was
// refused.
type OutboundImportItemError struct {
	// Kind is an OutboundImportItem* constant naming the item's list.
	Kind string `json:"kind"`
	// Index is the item's position in its list; -1 for a limit.
	Index int `json:"index"`
	// Key names the item: event type name, consumer ID, or an endpoint's
	// consumer and URL. Secrets and credentials are never included.
	Key   string `json:"key,omitempty"`
	Error string `json:"error"`
}

// OutboundImportCounts are the items of one kind an import created and the
// ones that already existed and were left unchanged.
type OutboundImportCounts struct {
	Total     int `json:"total"`
	Created   int `json:"created"`
	Unchanged int `json:"unchanged"`
}

// Outbound import statuses.
const (
	// OutboundImportStatusPending: accepted, waiting for a worker.
	OutboundImportStatusPending = "pending"
	// OutboundImportStatusValidating: checking every item again, including
	// that endpoint hosts are public, before anything is written.
	OutboundImportStatusValidating = "validating"
	// OutboundImportStatusRunning: creating items.
	OutboundImportStatusRunning   = "running"
	OutboundImportStatusCompleted = "completed"
	OutboundImportStatusFailed    = "failed"
)

// OutboundImport is the progress of a bulk import.
type OutboundImport struct {
	ID string `json:"id"`
	// Status is an OutboundImportStatus* constant.
	Status string `json:"status"`
	// Total and Processed count all items; Processed advances as each item is
	// created or found unchanged.
	Total      int                  `json:"total"`
	Processed  int                  `json:"processed"`
	EventTypes OutboundImportCounts `json:"event_types"`
	Consumers  OutboundImportCounts `json:"consumers"`
	Endpoints  OutboundImportCounts `json:"endpoints"`
	// Errors are the items that failed validation in the background (an
	// endpoint host that resolves to a private address); nothing was written
	// then.
	Errors      []OutboundImportItemError `json:"errors,omitempty"`
	Error       string                    `json:"error,omitempty"`
	CreatedAt   time.Time                 `json:"created_at"`
	UpdatedAt   time.Time                 `json:"updated_at"`
	CompletedAt *time.Time                `json:"completed_at,omitempty"`
}

// Done reports whether the import has finished, completed or failed.
func (i *OutboundImport) Done() bool {
	return i.Status == OutboundImportStatusCompleted || i.Status == OutboundImportStatusFailed
}

// StartOutboundImport validates the import and starts it in the background;
// poll GetOutboundImport until it is Done. Up to OutboundMaxImportItems items
// per import, one import at a time per account.
//
// Invalid items refuse the whole import with a 400 APIError whose ImportErrors
// list each item (nothing is written); another running import answers 409
// (IsConflict) and a body over OutboundMaxImportBytes answers 413.
func (api *API) StartOutboundImport(options *OutboundImportOptions) (*OutboundImport, error) {
	var task OutboundImport
	if err := api.outbound(http.MethodPost, outboundPath("imports"), options, &task, nil); err != nil {
		return nil, err
	}
	return &task, nil
}

// GetOutboundImport returns an import's progress.
func (api *API) GetOutboundImport(id string) (*OutboundImport, error) {
	var task OutboundImport
	if err := api.outbound(http.MethodGet, outboundPath("imports", id), nil, &task, nil); err != nil {
		return nil, err
	}
	return &task, nil
}
