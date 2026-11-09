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
	// Both caps mirror the API's limits at this release; the API's 400 and
	// 413 answers are authoritative.
	OutboundMaxImportItems = 5000
	// OutboundMaxImportBytes caps an import's request body; larger bodies are
	// refused with a 413 APIError.
	OutboundMaxImportBytes = 10 << 20
)

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
	// Example is any value that encodes to JSON; pass raw JSON bytes as a
	// json.RawMessage, since a []byte encodes as a base64 string.
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
	// Svix) so the consumer keeps verifying with it: "whsec_" + 32-100 base64
	// characters decoding to at least 24 bytes. Omit it to generate one.
	// Write-only, never returned; an invalid one is refused without echoing
	// it.
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
	// Kind is an OutboundImportItem* constant naming the item's list, or a
	// kind the API added after this release.
	Kind string `json:"kind"`
	// Index is the item's position in its list; -1 for a limit.
	Index int `json:"index"`
	// Key names the item: event type name, consumer ID, or an endpoint's
	// consumer and URL. For a limit it names the list (event_types,
	// consumers) or the consumer. Signing secrets and cloud credentials are
	// never included, but an endpoint's URL may itself be sensitive.
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

// OutboundImportTask is the progress of a bulk import.
type OutboundImportTask struct {
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
	// Errors are the items the background validation refused (an endpoint
	// host that resolves to a private address, a Function deleted since the
	// import started); nothing was written then.
	Errors []OutboundImportItemError `json:"errors,omitempty"`
	// Error says why a failed import stopped. When Errors is empty, the
	// items before the one that stopped it may already exist; starting the
	// same import again resumes it.
	Error       string     `json:"error,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}

// Done reports whether the import has finished. Any status but pending,
// validating and running is final, including ones added after this release;
// a task without a status is not.
func (i *OutboundImportTask) Done() bool {
	switch i.Status {
	case "", OutboundImportStatusPending, OutboundImportStatusValidating, OutboundImportStatusRunning:
		return false
	}
	return true
}

// StartOutboundImport validates the import and starts it in the background;
// poll GetOutboundImport every few seconds until it is Done
// (examples/outbound ImportCustomers shows the whole flow). Up to
// OutboundMaxImportItems items per import, one import at a time per account.
//
// Invalid items refuse the whole import with a 400 APIError whose
// ImportErrors list up to 100 of them (nothing is written; Message gives the
// total). An empty or oversized import is refused with Message alone. For a
// 400 the *APIError's Error() is the raw body (the returned error wraps it),
// item keys and their endpoint URLs included. Sending the same import again
// while it runs returns the running task, so the client's automatic retries
// are safe; if it already finished, a retry starts a new import, which finds
// the items of a completed one unchanged and resumes a failed one. A different
// import answers 409 (IsConflict) until the running one finishes, and a body
// over OutboundMaxImportBytes answers 413. A large import can take a while to
// upload and validate: bound it with WithContext.
func (api *API) StartOutboundImport(options *OutboundImportOptions) (*OutboundImportTask, error) {
	if options == nil {
		options = &OutboundImportOptions{}
	}
	var task OutboundImportTask
	if err := api.outbound(http.MethodPost, outboundPath("imports"), options, &task, nil); err != nil {
		return nil, err
	}
	return &task, nil
}

// GetOutboundImport returns an import's progress.
func (api *API) GetOutboundImport(id string) (*OutboundImportTask, error) {
	var task OutboundImportTask
	if err := api.outbound(http.MethodGet, outboundPath("imports", id), nil, &task, nil); err != nil {
		return nil, err
	}
	return &task, nil
}
