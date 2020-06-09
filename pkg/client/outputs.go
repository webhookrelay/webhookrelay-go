package client

import (
	"encoding/json"
	"fmt"
	"time"
)

// Output specified webhook forwarding destination
type Output struct {
	ID          string              `json:"id"`
	CreatedAt   time.Time           `json:"created_at"`
	UpdatedAt   time.Time           `json:"updated_at"`
	Name        string              `json:"name"`
	BucketID    string              `json:"bucket_id"`
	FunctionID  string              `json:"function_id"`
	Headers     map[string][]string `json:"headers"`
	Destination string              `json:"destination"`
	Internal    bool                `json:"internal"`
	Timeout     int                 `json:"timeout"` // Destination response timeout
	Description string              `json:"description"`
}

// MarshalJSON helper to change time into unix
func (o *Output) MarshalJSON() ([]byte, error) {
	type Alias Output
	return json.Marshal(&struct {
		CreatedAt int64 `json:"created_at"`
		UpdatedAt int64 `json:"updated_at"`
		*Alias
	}{
		CreatedAt: o.CreatedAt.Unix(),
		UpdatedAt: o.UpdatedAt.Unix(),
		Alias:     (*Alias)(o),
	})
}

// UnmarshalJSON helper to change time from unix
func (o *Output) UnmarshalJSON(data []byte) error {
	type Alias Output
	aux := &struct {
		CreatedAt int64 `json:"created_at"`
		UpdatedAt int64 `json:"updated_at"`
		*Alias
	}{
		Alias: (*Alias)(o),
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	o.CreatedAt = time.Unix(aux.CreatedAt, 0)
	o.UpdatedAt = time.Unix(aux.UpdatedAt, 0)
	return nil
}

// OutputListOptions used to query outputs
type OutputListOptions struct {
	Bucket string // Bucket reference - ID or name
}

// OutputList returns a list of outputs belonging to the bucket. If bucket reference not supplied,
// all account outputs will be returned
func (api *API) OutputList(options *OutputListOptions) ([]*Output, error) {
	if options.Bucket == "" {
		return api.allOutputList(&BucketListOptions{})
	}

	bucket, err := api.GetBucket(options.Bucket)
	if err != nil {
		return nil, err
	}

	var outputs []*Output
	for idx := range bucket.Outputs {
		outputs = append(outputs, &bucket.Outputs[idx])
	}

	return outputs, nil
}

func (api *API) allOutputList(options *BucketListOptions) ([]*Output, error) {
	buckets, err := api.ListBuckets(options)
	if err != nil {
		return nil, fmt.Errorf("failed to get outputs, error: %w", err)
	}

	var outputs []*Output
	for idx := range buckets {
		for bIdx := range buckets[idx].Outputs {
			outputs = append(outputs, &buckets[idx].Outputs[bIdx])
		}
	}

	return outputs, nil
}
