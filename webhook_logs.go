//go:generate jsonenums -type=RequestStatus
package webhookrelay

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/pkg/errors"
)

// default statuses
const (
	RequestStatusPreparing RequestStatus = iota
	RequestStatusSent
	RequestStatusFailed
	RequestStatusStalled // if request destination wasn't listening - incoming requests will be stalled
	RequestStatusReceived
	RequestStatusRejected
)

// RequestStatus - available request statuses
type RequestStatus int

func (s RequestStatus) String() string {
	switch s {
	case RequestStatusPreparing:
		return "preparing"
	case RequestStatusSent:
		return "sent"
	case RequestStatusFailed:
		return "failed"
	case RequestStatusStalled:
		return "stalled"
	case RequestStatusReceived:
		return "received"
	case RequestStatusRejected:
		return "rejected"
	default:
		return "unknown"
	}
}

// Log - stination defines where particular request was going with
// output ID and additional stats
type Log struct {
	ID        string    `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	AccountID string `json:"account_id"`
	OutputID  string `json:"output_id"`
	InputID   string `json:"input_id"`
	BucketID  string `json:"bucket_id"`

	Internal        bool          `json:"internal"`
	StatusCode      int           `json:"status_code"`
	ResponseBody    []byte        `json:"response_body"`
	ResponseHeaders Headers       `json:"response_headers" `
	Status          RequestStatus `json:"status"`
	Retries         int           `json:"retries"`

	// request details
	Headers   Headers `json:"headers"`
	RawQuery  string  `json:"raw_query"`
	Method    string  `json:"method"`
	ExtraPath string  `json:"extra_path"`

	Body string `json:"body"`

	// If true, doesn't save request body, query or headers,
	// inherits from the bucket configuration
	Ephemeral bool `json:"ephemeral"`
}

// Headers - headers are used to store request header info in the webhook log
type Headers map[string]interface{}

// MarshalJSON converst Go time into unix time
func (l *Log) MarshalJSON() ([]byte, error) {
	type Alias Log
	return json.Marshal(&struct {
		CreatedAt int64 `json:"created_at"`
		UpdatedAt int64 `json:"updated_at"`
		*Alias
	}{
		CreatedAt: l.CreatedAt.Unix(),
		UpdatedAt: l.UpdatedAt.Unix(),
		Alias:     (*Alias)(l),
	})
}

// UnmarshalJSON parses unix time
func (l *Log) UnmarshalJSON(data []byte) error {
	type Alias Log
	aux := &struct {
		CreatedAt int64 `json:"created_at"`
		UpdatedAt int64 `json:"updated_at"`
		*Alias
	}{
		Alias: (*Alias)(l),
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	l.CreatedAt = time.Unix(aux.CreatedAt, 0)
	l.UpdatedAt = time.Unix(aux.UpdatedAt, 0)
	return nil
}

// WebhookLogsListOptions - list logs options
type WebhookLogsListOptions struct {
	BucketID string
	Status   RequestStatus
	From     time.Time
	To       time.Time
	Limit    int
	Offset   int
}

// WebhookLogsResponse is a webhook query response
type WebhookLogsResponse struct {
	Data   []*Log `json:"data"`
	Total  int    `json:"total"`
	Limit  int    `json:"limit"`
	Offset int    `json:"offset"`
}

// ListWebhookLogs lists webhook logs for an account
func (api *API) ListWebhookLogs(options *WebhookLogsListOptions) (*WebhookLogsResponse, error) {

	resp, err := api.makeRequest(http.MethodGet, "/logs", nil)
	if err != nil {
		return nil, errors.Wrap(err, errMakeRequestError)
	}

	var webhookLogs WebhookLogsResponse
	err = json.Unmarshal(resp, &webhookLogs)
	if err != nil {
		return nil, errors.Wrap(err, errUnmarshalError)
	}

	return &webhookLogs, nil
}

func getQuery(options *WebhookLogsListOptions) string {
	u := url.URL{}

	q := u.Query()

	q.Add("bucket", options.BucketID)
	q.Add("status", options.Status.String())
	q.Add("limit", strconv.Itoa(options.Limit))
	q.Add("offset", strconv.Itoa(options.Offset))

	return q.Encode()
}

// GetWebhookLog - returns webhook lgo
func (api *API) GetWebhookLog(id string) (*Log, error) {

	resp, err := api.makeRequest(http.MethodGet, "/logs/"+id, nil)
	if err != nil {
		return nil, errors.Wrap(err, errMakeRequestError)
	}

	var webhookLog Log
	err = json.Unmarshal(resp, &webhookLog)
	if err != nil {
		return nil, errors.Wrap(err, errUnmarshalError)
	}

	return &webhookLog, nil
}
