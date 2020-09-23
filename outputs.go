package webhookrelay

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/pkg/errors"
)

var (
	// ErrNoSuchOutput is the error returned when the Output does not exist.
	ErrNoSuchOutput = errors.New("no such output")
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
	Disabled    bool                `json:"disabled"` // Allows disabling forwarding to specific output
	// LockPath ensures that the request path cannot be changed from what is
	// specified in the destination. For example if request is coming to /v1/webhooks/xxx/github-jenkins,
	// with lock path 'false' and destination 'http://localhost:8080' it would go to http://localhost:8080/github-jenkins.
	// However, with lock path 'true', it will be sent to 'http://localhost:8080'
	LockPath    bool   `json:"lock_path"`
	Internal    bool   `json:"internal"`
	Timeout     int    `json:"timeout"` // Destination response timeout
	Description string `json:"description"`
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

// ListOutputs returns a list of outputs belonging to the bucket. If bucket reference not supplied,
// all account outputs will be returned
func (api *API) ListOutputs(options *OutputListOptions) ([]*Output, error) {
	if options.Bucket == "" {
		return api.allOutputList(&BucketListOptions{})
	}

	bucket, err := api.GetBucket(options.Bucket)
	if err != nil {
		return nil, err
	}

	var outputs []*Output
	for idx := range bucket.Outputs {
		outputs = append(outputs, bucket.Outputs[idx])
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
			outputs = append(outputs, buckets[idx].Outputs[bIdx])
		}
	}

	return outputs, nil
}

// CreateOutput creates an Output and returns the new object
func (api *API) CreateOutput(options *Output) (*Output, error) {
	bucketID, err := api.ensureBucketID(options.BucketID)
	if err != nil {
		return nil, err
	}

	resp, err := api.makeRequest("POST", "/buckets/"+bucketID+"/outputs", options)
	if err != nil {
		return nil, err
	}

	var result Output
	err = json.Unmarshal(resp, &result)
	return &result, nil
}

// UpdateOutput updates output
func (api *API) UpdateOutput(options *Output) (*Output, error) {

	bucketID, err := api.ensureBucketID(options.BucketID)
	if err != nil {
		return nil, err
	}

	outputID, err := api.ensureOutputID(bucketID, options.ID)
	if err != nil {
		return nil, err
	}

	resp, err := api.makeRequest("PUT", "/buckets/"+bucketID+"/outputs/"+outputID, options)
	if err != nil {
		return nil, err
	}

	var output Output
	err = json.Unmarshal(resp, &output)
	return &output, nil
}

// OutputDeleteOptions delete options
type OutputDeleteOptions struct {
	Bucket string
	Output string // ID or name
}

// DeleteOutput deletes output from the bucket
func (api *API) DeleteOutput(options *OutputDeleteOptions) error {

	if options.Bucket == "" {
		return fmt.Errorf("bucket not specified")
	}

	if options.Output == "" {
		return fmt.Errorf("output not specified")
	}

	bucketID, err := api.ensureBucketID(options.Bucket)
	if err != nil {
		return err
	}

	outputID, err := api.ensureOutputID(bucketID, options.Output)
	if err != nil {
		return err
	}

	_, err = api.makeRequest("DELETE", "/buckets/"+bucketID+"/outputs/"+outputID, nil)
	return err
}

func (api *API) ensureOutputID(bucket, ref string) (string, error) {
	if !IsUUID(ref) {
		id, err := api.outputIDFromName(bucket, ref)
		if err != nil {
			return "", err
		}
		return id, nil
	}
	return ref, nil
}

func (api *API) outputIDFromName(bucket, name string) (id string, err error) {
	outputs, err := api.ListOutputs(&OutputListOptions{
		Bucket: bucket,
	})
	if err != nil {
		return
	}
	for _, b := range outputs {
		if b.Name == name {
			return b.ID, nil
		}
	}
	return "", ErrNoSuchOutput
}
