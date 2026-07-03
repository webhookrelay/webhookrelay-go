package webhookrelay

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/pkg/errors"
)

// Cron is a scheduled webhook trigger. When it fires, the configured request
// (method, payload, headers) is injected into the cron's auto-created bucket and
// relayed to the destination through the bucket's outputs. Creating a cron
// server-side also provisions a bucket and an output; the SDK only needs to
// manage the Cron resource itself.
type Cron struct {
	ID        string    `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	Name string `json:"name"`

	// Enabled toggles whether the schedule is active.
	Enabled bool `json:"enabled"`
	// Recurring marks the job as recurring (the scheduler currently treats all
	// jobs as recurring).
	Recurring bool `json:"recurring"`
	// Schedule is a standard 5-field cron expression (minute hour dom month dow)
	// or a predefined descriptor such as "@daily" / "@every 1h30m". Do not embed
	// a TZ= prefix — set Timezone instead.
	Schedule string `json:"schedule"`
	// Timezone is an IANA timezone, e.g. "UTC" or "America/New_York".
	Timezone string `json:"timezone"`

	AccountID string `json:"account_id"`

	// Bucket is the cron's auto-created bucket. It is populated on get/list
	// responses; clients do not need to set it on create/update.
	Bucket Bucket `json:"bucket"`

	// Method is the HTTP method used for the fired webhook (GET, POST, ...).
	Method string `json:"method"`
	// Payload is the request body sent when the cron fires.
	Payload string `json:"payload"`
	// Headers are injected into the fired request.
	Headers map[string][]string `json:"headers"`
	// Destination is the target URL the webhook is delivered to.
	Destination string `json:"destination"`
	// FunctionID optionally attaches a transformation function to the output.
	FunctionID string `json:"function_id"`

	// StartsAt / EndsAt bound the active window. Leave as the zero time for no
	// lower/upper bound.
	StartsAt time.Time `json:"starts_at"`
	EndsAt   time.Time `json:"ends_at"`

	// NextRun is the computed next run time. It is populated only by ListCrons
	// and is read-only.
	NextRun time.Time `json:"next_run"`
}

// MarshalJSON emits only client-settable fields. Read-only fields (bucket,
// next_run, created_at, updated_at) are omitted, and zero StartsAt/EndsAt are
// omitted rather than serialized as year-1 timestamps — the API treats an
// absent bound as "no bound".
func (c *Cron) MarshalJSON() ([]byte, error) {
	type Alias Cron
	aux := &struct {
		Bucket    *Bucket    `json:"bucket,omitempty"`
		NextRun   *time.Time `json:"next_run,omitempty"`
		CreatedAt *time.Time `json:"created_at,omitempty"`
		UpdatedAt *time.Time `json:"updated_at,omitempty"`
		StartsAt  *time.Time `json:"starts_at,omitempty"`
		EndsAt    *time.Time `json:"ends_at,omitempty"`
		*Alias
	}{
		Alias: (*Alias)(c),
	}
	if !c.StartsAt.IsZero() {
		aux.StartsAt = &c.StartsAt
	}
	if !c.EndsAt.IsZero() {
		aux.EndsAt = &c.EndsAt
	}
	return json.Marshal(aux)
}

// UnmarshalJSON accepts either unix seconds or RFC3339 strings for every time
// field, so responses decode regardless of the format the API uses.
func (c *Cron) UnmarshalJSON(data []byte) error {
	type Alias Cron
	aux := &struct {
		CreatedAt json.RawMessage `json:"created_at"`
		UpdatedAt json.RawMessage `json:"updated_at"`
		StartsAt  json.RawMessage `json:"starts_at"`
		EndsAt    json.RawMessage `json:"ends_at"`
		NextRun   json.RawMessage `json:"next_run"`
		*Alias
	}{
		Alias: (*Alias)(c),
	}
	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}
	var err error
	if c.CreatedAt, err = parseTime(aux.CreatedAt); err != nil {
		return err
	}
	if c.UpdatedAt, err = parseTime(aux.UpdatedAt); err != nil {
		return err
	}
	if c.StartsAt, err = parseTime(aux.StartsAt); err != nil {
		return err
	}
	if c.EndsAt, err = parseTime(aux.EndsAt); err != nil {
		return err
	}
	if c.NextRun, err = parseTime(aux.NextRun); err != nil {
		return err
	}
	return nil
}

// CronListOptions is used to list crons.
type CronListOptions struct{}

// ListCrons lists cron triggers for an account.
func (api *API) ListCrons(options *CronListOptions) ([]*Cron, error) {
	resp, err := api.makeRequest(http.MethodGet, "/crons", nil)
	if err != nil {
		return nil, errors.Wrap(err, errMakeRequestError)
	}

	var crons []*Cron
	if err := json.Unmarshal(resp, &crons); err != nil {
		return nil, errors.Wrap(err, errUnmarshalError)
	}

	return crons, nil
}

// GetCron gets a cron by ID.
func (api *API) GetCron(id string) (*Cron, error) {
	resp, err := api.makeRequest(http.MethodGet, "/crons/"+id, nil)
	if err != nil {
		return nil, err
	}

	var cron Cron
	if err := json.Unmarshal(resp, &cron); err != nil {
		return nil, err
	}
	return &cron, nil
}

// CreateCron creates a new cron trigger.
func (api *API) CreateCron(options *Cron) (*Cron, error) {
	resp, err := api.makeRequest(http.MethodPost, "/crons", options)
	if err != nil {
		return nil, err
	}

	var cron Cron
	if err := json.Unmarshal(resp, &cron); err != nil {
		return nil, err
	}
	return &cron, nil
}

// UpdateCron updates an existing cron trigger. The cron ID must be set.
func (api *API) UpdateCron(options *Cron) (*Cron, error) {
	if options.ID == "" {
		return nil, fmt.Errorf("cron ID must be supplied")
	}

	resp, err := api.makeRequest(http.MethodPut, "/crons/"+options.ID, options)
	if err != nil {
		return nil, err
	}

	var cron Cron
	if err := json.Unmarshal(resp, &cron); err != nil {
		return nil, err
	}
	return &cron, nil
}

// DeleteCron deletes a cron by ID.
func (api *API) DeleteCron(id string) error {
	if id == "" {
		return fmt.Errorf("cron ID must be supplied")
	}

	_, err := api.makeRequest(http.MethodDelete, "/crons/"+id, nil)
	return err
}
