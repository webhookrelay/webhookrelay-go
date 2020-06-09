package client

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/pkg/errors"
)

var (
	// ErrNoSuchInput is the error returned when the Input does not exist.
	ErrNoSuchInput = errors.New("no such input")
)

// Input - webhook inputs are used to create endpoints which are then used
// by remote systems
type Input struct {
	ID         string              `json:"id"`
	CreatedAt  time.Time           `json:"created_at"`
	UpdatedAt  time.Time           `json:"updated_at"`
	Name       string              `json:"name"`
	FunctionID string              `json:"function_id"`
	BucketID   string              `json:"bucket_id"`
	Headers    map[string][]string `json:"headers"`
	StatusCode int                 `json:"status_code"`
	Body       string              `json:"body"`
	// either output ID or "anyOutput" to indicate that the first response
	// from any output is good enough. Empty string
	ResponseFromOutput string `json:"response_from_output"`
	CustomDomain       string `json:"custom_domain"`
	PathPrefix         string `json:"path_prefix"`
	Description        string `json:"description"`
}

// MarshalJSON helper to change time into unix
func (i *Input) MarshalJSON() ([]byte, error) {
	type Alias Input
	return json.Marshal(&struct {
		CreatedAt int64 `json:"created_at"`
		UpdatedAt int64 `json:"updated_at"`
		*Alias
	}{
		CreatedAt: i.CreatedAt.Unix(),
		UpdatedAt: i.UpdatedAt.Unix(),
		Alias:     (*Alias)(i),
	})
}

// UnmarshalJSON helper to change time from unix
func (i *Input) UnmarshalJSON(data []byte) error {
	type Alias Input
	aux := &struct {
		CreatedAt int64 `json:"created_at"`
		UpdatedAt int64 `json:"updated_at"`
		*Alias
	}{
		Alias: (*Alias)(i),
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	i.CreatedAt = time.Unix(aux.CreatedAt, 0)
	i.UpdatedAt = time.Unix(aux.UpdatedAt, 0)
	return nil
}

type InputListOptions struct {
	Bucket string // Bucket reference - ID or name
}

// ListInputs returns a list of inputs belonging to the bucket. If bucket reference not supplied,
// all account inputs will be returned
func (api *API) ListInputs(options *InputListOptions) ([]*Input, error) {

	if options.Bucket == "" {
		return api.allInputList(&BucketListOptions{})
	}

	bucket, err := api.GetBucket(options.Bucket)
	if err != nil {
		return nil, err
	}

	var inputs []*Input
	for idx := range bucket.Inputs {
		inputs = append(inputs, &bucket.Inputs[idx])
	}

	return inputs, nil
}

func (api *API) allInputList(opts *BucketListOptions) ([]*Input, error) {
	buckets, err := api.ListBuckets(opts)
	if err != nil {
		return nil, fmt.Errorf("failed to get inputs, error: %w", err)
	}

	var inputs []*Input
	for idx := range buckets {
		for bIdx := range buckets[idx].Inputs {
			inputs = append(inputs, &buckets[idx].Inputs[bIdx])
		}
	}

	return inputs, nil
}

// CreateInput creates a Input and returns the new object.
func (api *API) CreateInput(options *Input) (*Input, error) {
	bucketID, err := api.ensureBucketID(options.BucketID)
	if err != nil {
		return nil, err
	}

	resp, err := api.makeRequest("POST", "/buckets/"+bucketID+"/inputs", options)
	if err != nil {
		return nil, err
	}

	var input Input
	err = json.Unmarshal(resp, &input)
	return &input, nil
}

// UpdateInput updates existing input
func (api *API) UpdateInput(options *Input) (*Input, error) {
	if options.BucketID == "" {
		return nil, fmt.Errorf("bucket not specified")
	}

	if options.ID == "" && options.Name == "" {
		return nil, fmt.Errorf("either input ID or name has to be specified")
	}

	bucketID, err := api.ensureBucketID(options.BucketID)
	if err != nil {
		return nil, err
	}

	inputID, err := api.ensureInputID(options.Name)
	if err != nil {
		return nil, err
	}

	resp, err := api.makeRequest("PUT", "/buckets/"+bucketID+"/inputs/"+inputID, options)
	if err != nil {
		return nil, err
	}

	var input Input
	err = json.Unmarshal(resp, &input)
	return &input, nil
}

// InputDeleteOptions delete options
type InputDeleteOptions struct {
	Bucket string
	Input  string
}

// DeleteInput removes input. If public input is used by the UUID, beware that after deleting
// an input you will not be able to recreate another one with the same ID.
func (api *API) DeleteInput(options *InputDeleteOptions) error {

	if options.Bucket == "" {
		return fmt.Errorf("bucket not specified")
	}

	if options.Input == "" {
		return fmt.Errorf("input not specified")
	}

	bucketID, err := api.ensureBucketID(options.Bucket)
	if err != nil {
		return err
	}

	inputID, err := api.ensureInputID(options.Input)
	if err != nil {
		return err
	}

	_, err = api.makeRequest("DELETE", "/buckets/"+bucketID+"/inputs/"+inputID, nil)
	return err
}

// ensureInputID - takes name/id and always returns ID (when it not fails)
func (api *API) ensureInputID(ref string) (string, error) {
	if !IsUUID(ref) {
		id, err := api.inputIDFromName(ref)
		if err != nil {
			return "", err
		}
		return id, nil
	}
	return ref, nil
}

func (api *API) inputIDFromName(name string) (id string, err error) {
	inputs, err := api.ListInputs(&InputListOptions{})
	if err != nil {
		return
	}
	for _, b := range inputs {
		if b.Name == name {
			return b.ID, nil
		}
	}
	return "", ErrNoSuchInput
}
