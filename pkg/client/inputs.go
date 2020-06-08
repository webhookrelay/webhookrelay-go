package client

import (
	"encoding/json"
	"fmt"
	"time"
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
