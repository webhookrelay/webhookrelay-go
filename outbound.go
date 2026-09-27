package webhookrelay

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"
)

// Outbound webhooks let you send signed webhooks to your own customers: register
// each customer as a consumer with the HTTPS endpoints they give you, describe
// your events as event types, then publish messages. Webhook Relay signs each
// delivery (Standard Webhooks), retries failures durably and keeps the
// delivery history.
//
// Outbound webhooks are in pilot: the account needs the "outbound" feature.

// OutboundConsumer is one of your customers. You choose its ID, for example
// your own customer ID.
type OutboundConsumer struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// Rate is the default deliveries per second for the consumer's endpoints;
	// 0 means 50.
	Rate      int       `json:"rate"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// OutboundConsumerOptions creates or updates a consumer.
type OutboundConsumerOptions struct {
	ID   string `json:"-"`
	Name string `json:"name,omitempty"`
	Rate int    `json:"rate,omitempty"`
}

// OutboundEventType is an entry in your event catalog, such as "invoice.paid".
type OutboundEventType struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Example     json.RawMessage `json:"example,omitempty"`
	// Deprecated event types cannot be published.
	Deprecated bool      `json:"deprecated"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// OutboundEventTypeOptions creates or updates an event type.
type OutboundEventTypeOptions struct {
	Name        string      `json:"-"`
	Description string      `json:"description,omitempty"`
	Example     interface{} `json:"example,omitempty"`
	Deprecated  bool        `json:"deprecated,omitempty"`
}

// Outbound endpoint states.
const (
	OutboundEndpointStateActive   = "active"
	OutboundEndpointStateFailing  = "failing"
	OutboundEndpointStatePaused   = "paused"
	OutboundEndpointStateDisabled = "disabled"
)

// OutboundEndpoint is a consumer's HTTPS destination.
type OutboundEndpoint struct {
	ID          string            `json:"id"`
	Consumer    string            `json:"consumer"`
	URL         string            `json:"url"`
	Description string            `json:"description"`
	EventTypes  []string          `json:"event_types"`
	Headers     map[string]string `json:"headers,omitempty"`
	Rate        int               `json:"rate"`
	// Timeout is seconds per delivery attempt; 0 means 15.
	Timeout             int        `json:"timeout"`
	FunctionID          string     `json:"function_id,omitempty"`
	AutoDisable         bool       `json:"auto_disable"`
	State               string     `json:"state"`
	ConsecutiveFailures int        `json:"consecutive_failures"`
	FailingSince        *time.Time `json:"failing_since,omitempty"`
	// PreviousSecretExpiresAt is when the secret replaced by the last rotation
	// stops signing deliveries.
	PreviousSecretExpiresAt *time.Time `json:"previous_secret_expires_at,omitempty"`
	CreatedAt               time.Time  `json:"created_at"`
	UpdatedAt               time.Time  `json:"updated_at"`

	// Secret is the signing secret. It is only set on the endpoint returned by
	// CreateOutboundEndpoint.
	Secret string `json:"secret,omitempty"`
}

// OutboundEndpointOptions configures an endpoint.
type OutboundEndpointOptions struct {
	// Consumer is required when creating an endpoint.
	Consumer string `json:"-"`
	// URL is a public HTTPS URL.
	URL string `json:"url"`
	// EventTypes lists the event types to deliver, or []string{"*"} for all.
	EventTypes  []string          `json:"event_types"`
	Description string            `json:"description,omitempty"`
	Headers     map[string]string `json:"headers,omitempty"`
	// Rate is deliveries per second; 0 uses the consumer's rate.
	Rate int `json:"rate,omitempty"`
	// Timeout is seconds per attempt, up to 60; 0 means 15.
	Timeout int `json:"timeout,omitempty"`
	// AutoDisable disables the endpoint after 100 consecutive failed
	// deliveries over at least 12 hours.
	AutoDisable bool `json:"auto_disable,omitempty"`
	// FunctionID attaches a Function that can rewrite the JSON body or drop
	// the message.
	FunctionID string `json:"function_id,omitempty"`
}

// OutboundMessage is an accepted message.
type OutboundMessage struct {
	ID          string          `json:"id"`
	Consumer    string          `json:"consumer"`
	EventType   string          `json:"event_type"`
	EventID     string          `json:"event_id,omitempty"`
	Payload     json.RawMessage `json:"payload,omitempty"`
	EndpointIDs []string        `json:"endpoint_ids"`
	CreatedAt   time.Time       `json:"created_at"`
	// EnqueuedAt is when deliveries were queued; nil while the message is
	// still being prepared.
	EnqueuedAt *time.Time `json:"enqueued_at,omitempty"`
	// Deliveries are set by GetOutboundMessage: one per addressed endpoint.
	Deliveries []*Log `json:"deliveries,omitempty"`
	// UnavailableEndpointIDs are set by GetOutboundMessage for endpoints whose
	// delivery could not be read in time; ask again later.
	UnavailableEndpointIDs []string `json:"unavailable_endpoint_ids,omitempty"`
}

// OutboundMessagePublishOptions publishes a message.
type OutboundMessagePublishOptions struct {
	Consumer  string `json:"consumer"`
	EventType string `json:"event_type"`
	// EventID is your own event ID, delivered with the message.
	EventID string `json:"event_id,omitempty"`
	// Payload is any value that encodes to JSON, up to 256 KiB.
	Payload interface{} `json:"payload"`
	// IdempotencyKey makes publishing safe to retry: repeating a publish with
	// the same key and message returns the original message. When empty, the
	// client generates one per call, so its own automatic retries cannot
	// publish twice.
	IdempotencyKey string `json:"-"`
}

// OutboundMessageListOptions filters and pages messages.
type OutboundMessageListOptions struct {
	Consumer  string
	EventType string
	Limit     int
	Offset    int
}

// OutboundDeliveryListOptions pages an endpoint's deliveries.
type OutboundDeliveryListOptions struct {
	EndpointID string
	Limit      int
	Offset     int
	// Status keeps deliveries in one status: sent, failed, stalled (a retry
	// is scheduled), received (queued) or rejected (skipped).
	Status string
	// EventType keeps deliveries of one event type.
	EventType string
	// MessageID keeps the delivery of one message.
	MessageID string
}

// OutboundRecoveryTask is the progress of a background re-send.
type OutboundRecoveryTask struct {
	ID         string    `json:"id"`
	EndpointID string    `json:"endpoint_id"`
	MessageID  string    `json:"message_id,omitempty"`
	Kind       string    `json:"kind"`   // retry, recover or replay-missing
	Status     string    `json:"status"` // pending, running, completed or failed
	Since      time.Time `json:"since"`
	Until      time.Time `json:"until"`
	Processed  int       `json:"processed"`
	Error      string    `json:"error,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// ListOutboundConsumers returns your consumers.
func (api *API) ListOutboundConsumers() ([]*OutboundConsumer, error) {
	var consumers []*OutboundConsumer
	err := api.outbound(http.MethodGet, outboundPath("consumers"), nil, &consumers, nil)
	return consumers, err
}

// GetOutboundConsumer returns a consumer by ID.
func (api *API) GetOutboundConsumer(id string) (*OutboundConsumer, error) {
	var consumer OutboundConsumer
	if err := api.outbound(http.MethodGet, outboundPath("consumers", id), nil, &consumer, nil); err != nil {
		return nil, err
	}
	return &consumer, nil
}

// UpsertOutboundConsumer creates the consumer or updates its name and rate. A
// deleted consumer's ID cannot be reused.
func (api *API) UpsertOutboundConsumer(options *OutboundConsumerOptions) (*OutboundConsumer, error) {
	var consumer OutboundConsumer
	if err := api.outbound(http.MethodPut, outboundPath("consumers", options.ID), options, &consumer, nil); err != nil {
		return nil, err
	}
	return &consumer, nil
}

// DeleteOutboundConsumer deletes the consumer and its endpoints. Delivery
// history is kept.
func (api *API) DeleteOutboundConsumer(id string) error {
	return api.outbound(http.MethodDelete, outboundPath("consumers", id), nil, nil, nil)
}

// ListOutboundEventTypes returns your event catalog.
func (api *API) ListOutboundEventTypes() ([]*OutboundEventType, error) {
	var eventTypes []*OutboundEventType
	err := api.outbound(http.MethodGet, outboundPath("event-types"), nil, &eventTypes, nil)
	return eventTypes, err
}

// UpsertOutboundEventType creates the event type or updates its description,
// example and deprecation.
func (api *API) UpsertOutboundEventType(options *OutboundEventTypeOptions) (*OutboundEventType, error) {
	var eventType OutboundEventType
	if err := api.outbound(http.MethodPut, outboundPath("event-types", options.Name), options, &eventType, nil); err != nil {
		return nil, err
	}
	return &eventType, nil
}

// DeleteOutboundEventType deletes an event type no endpoint subscribes to.
func (api *API) DeleteOutboundEventType(name string) error {
	return api.outbound(http.MethodDelete, outboundPath("event-types", name), nil, nil, nil)
}

// ListOutboundEndpoints returns a consumer's endpoints, newest first.
func (api *API) ListOutboundEndpoints(consumer string) ([]*OutboundEndpoint, error) {
	var endpoints []*OutboundEndpoint
	err := api.outbound(http.MethodGet, outboundPath("consumers", consumer, "endpoints"), nil, &endpoints, nil)
	return endpoints, err
}

// GetOutboundEndpoint returns an endpoint by ID.
func (api *API) GetOutboundEndpoint(id string) (*OutboundEndpoint, error) {
	return api.outboundEndpoint(http.MethodGet, outboundPath("endpoints", id), nil)
}

// CreateOutboundEndpoint registers an HTTPS endpoint for options.Consumer. The
// returned endpoint carries its signing secret; share it with your customer
// so they can verify deliveries.
func (api *API) CreateOutboundEndpoint(options *OutboundEndpointOptions) (*OutboundEndpoint, error) {
	return api.outboundEndpoint(http.MethodPost, outboundPath("consumers", options.Consumer, "endpoints"), options)
}

// UpdateOutboundEndpoint replaces an endpoint's configuration. Its state and
// signing secrets are kept.
func (api *API) UpdateOutboundEndpoint(id string, options *OutboundEndpointOptions) (*OutboundEndpoint, error) {
	return api.outboundEndpoint(http.MethodPut, outboundPath("endpoints", id), options)
}

// DeleteOutboundEndpoint deletes an endpoint. Delivery history is kept.
func (api *API) DeleteOutboundEndpoint(id string) error {
	return api.outbound(http.MethodDelete, outboundPath("endpoints", id), nil, nil, nil)
}

// PauseOutboundEndpoint stops deliveries. Messages published while paused are
// recorded as skipped; send them later with ReplayMissingOutboundDeliveries.
func (api *API) PauseOutboundEndpoint(id string) (*OutboundEndpoint, error) {
	return api.outboundEndpoint(http.MethodPost, outboundPath("endpoints", id, "pause"), nil)
}

// ResumeOutboundEndpoint restarts deliveries and resets the failure count.
func (api *API) ResumeOutboundEndpoint(id string) (*OutboundEndpoint, error) {
	return api.outboundEndpoint(http.MethodPost, outboundPath("endpoints", id, "resume"), nil)
}

// RotateOutboundEndpointSecret replaces the signing secret and returns the new
// one. The previous secret keeps signing for 24 hours.
func (api *API) RotateOutboundEndpointSecret(id string) (string, error) {
	return api.outboundSecret(id, "rotate")
}

// RevealOutboundEndpointSecret returns the current signing secret.
func (api *API) RevealOutboundEndpointSecret(id string) (string, error) {
	return api.outboundSecret(id, "reveal")
}

// PublishOutboundMessage accepts a message for signed delivery to every
// endpoint of the consumer subscribed to its event type. Delivery is
// asynchronous: the returned message is the durable acceptance receipt.
func (api *API) PublishOutboundMessage(options *OutboundMessagePublishOptions) (*OutboundMessage, error) {
	key := options.IdempotencyKey
	if key == "" {
		var err error
		if key, err = newIdempotencyKey(); err != nil {
			return nil, err
		}
	}
	headers := http.Header{"Idempotency-Key": []string{key}}
	var message OutboundMessage
	if err := api.outbound(http.MethodPost, outboundPath("messages"), options, &message, headers); err != nil {
		return nil, err
	}
	return &message, nil
}

// GetOutboundMessage returns a message with its payload and one delivery per
// addressed endpoint, including attempts.
func (api *API) GetOutboundMessage(id string) (*OutboundMessage, error) {
	var message OutboundMessage
	if err := api.outbound(http.MethodGet, outboundPath("messages", id), nil, &message, nil); err != nil {
		return nil, err
	}
	return &message, nil
}

// ListOutboundMessages returns accepted messages, newest first, without
// payloads.
func (api *API) ListOutboundMessages(options *OutboundMessageListOptions) ([]*OutboundMessage, error) {
	if options == nil {
		options = &OutboundMessageListOptions{}
	}
	query := url.Values{}
	setQuery(query, "consumer", options.Consumer)
	setQuery(query, "event_type", options.EventType)
	setPage(query, options.Limit, options.Offset)
	var messages []*OutboundMessage
	err := api.outbound(http.MethodGet, withQuery(outboundPath("messages"), query), nil, &messages, nil)
	return messages, err
}

// ListOutboundDeliveries returns an endpoint's deliveries, newest first.
func (api *API) ListOutboundDeliveries(options *OutboundDeliveryListOptions) ([]*Log, error) {
	query := url.Values{}
	setPage(query, options.Limit, options.Offset)
	for key, value := range map[string]string{"status": options.Status, "event_type": options.EventType, "message_id": options.MessageID} {
		if value != "" {
			query.Set(key, value)
		}
	}
	path := withQuery(outboundPath("endpoints", options.EndpointID, "deliveries"), query)
	var deliveries []*Log
	err := api.outbound(http.MethodGet, path, nil, &deliveries, nil)
	return deliveries, err
}

// RetryOutboundDelivery re-sends one message to an endpoint in the background.
func (api *API) RetryOutboundDelivery(endpointID, messageID string) (*OutboundRecoveryTask, error) {
	return api.outboundRecovery(endpointID, "retry", outboundRecoveryBody{MessageID: messageID})
}

// RecoverOutboundDeliveries re-sends the endpoint's deliveries whose retries
// were exhausted since the given time (zero means the last 24 hours).
func (api *API) RecoverOutboundDeliveries(endpointID string, since time.Time) (*OutboundRecoveryTask, error) {
	return api.outboundRecovery(endpointID, "recover", outboundRecoveryBody{Since: optionalTime(since)})
}

// ReplayMissingOutboundDeliveries sends the deliveries skipped while the
// endpoint was paused or disabled, since the given time (zero means the last
// 24 hours). Resume the endpoint first.
func (api *API) ReplayMissingOutboundDeliveries(endpointID string, since time.Time) (*OutboundRecoveryTask, error) {
	return api.outboundRecovery(endpointID, "replay-missing", outboundRecoveryBody{Since: optionalTime(since)})
}

// GetOutboundRecoveryTask returns a recovery task's progress.
func (api *API) GetOutboundRecoveryTask(id string) (*OutboundRecoveryTask, error) {
	var task OutboundRecoveryTask
	if err := api.outbound(http.MethodGet, outboundPath("recovery-tasks", id), nil, &task, nil); err != nil {
		return nil, err
	}
	return &task, nil
}

type outboundRecoveryBody struct {
	MessageID string     `json:"message_id,omitempty"`
	Since     *time.Time `json:"since,omitempty"`
}

func (api *API) outboundRecovery(endpointID, kind string, body outboundRecoveryBody) (*OutboundRecoveryTask, error) {
	var task OutboundRecoveryTask
	if err := api.outbound(http.MethodPost, outboundPath("endpoints", endpointID, kind), body, &task, nil); err != nil {
		return nil, err
	}
	return &task, nil
}

func (api *API) outboundEndpoint(method, path string, options *OutboundEndpointOptions) (*OutboundEndpoint, error) {
	var params interface{}
	if options != nil {
		params = options
	}
	var endpoint OutboundEndpoint
	if err := api.outbound(method, path, params, &endpoint, nil); err != nil {
		return nil, err
	}
	return &endpoint, nil
}

func (api *API) outboundSecret(id, action string) (string, error) {
	var out struct {
		Secret string `json:"secret"`
	}
	if err := api.outbound(http.MethodPost, outboundPath("endpoints", id, "secret", action), nil, &out, nil); err != nil {
		return "", err
	}
	return out.Secret, nil
}

// outbound performs an outbound API request and decodes the response into out
// when it is not nil.
func (api *API) outbound(method, path string, params, out interface{}, headers http.Header) error {
	resp, err := api.makeRequestWithAuthTypeAndHeaders(api.requestContext(), method, path, params, api.authType, headers)
	if err != nil {
		return errors.Wrap(err, errMakeRequestError)
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(resp, out); err != nil {
		return errors.Wrap(err, errUnmarshalError)
	}
	return nil
}

// outboundPath joins escaped segments under /outbound.
func outboundPath(segments ...string) string {
	escaped := make([]string, len(segments))
	for i, segment := range segments {
		escaped[i] = url.PathEscape(segment)
	}
	return "/outbound/" + strings.Join(escaped, "/")
}

func withQuery(path string, query url.Values) string {
	if len(query) == 0 {
		return path
	}
	return path + "?" + query.Encode()
}

func setQuery(query url.Values, key, value string) {
	if value != "" {
		query.Set(key, value)
	}
}

func setPage(query url.Values, limit, offset int) {
	if limit > 0 {
		query.Set("limit", strconv.Itoa(limit))
	}
	if offset > 0 {
		query.Set("offset", strconv.Itoa(offset))
	}
}

func optionalTime(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

func newIdempotencyKey() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", errors.Wrap(err, "generating idempotency key")
	}
	return hex.EncodeToString(b), nil
}
