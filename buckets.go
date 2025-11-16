package webhookrelay

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/pkg/errors"
)

// Bucket - bucket is required for webhook inputs and outputs. There
// should probably be one Input per Bucket to make it easy to manage.
// Buckets control policies such as retries, manipulation, logs, rate limitting
type Bucket struct {
	ID          string    `json:"id"`         // readonly
	CreatedAt   time.Time `json:"created_at"` // readonly
	UpdatedAt   time.Time `json:"updated_at"` // readonly
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Stream      bool      `json:"stream"`
	Ephemeral   bool      `json:"ephemeral"`
	Suspended   bool      `json:"suspended"`
	// LargeWebhooks - if set, we allow larger than 3MB webhooks
	// to be sent to the bucket. This is useful for large file uploads
	LargeWebhooks bool       `json:"large_webhooks"`
	Auth          BucketAuth `json:"auth"`
	Inputs        []*Input   `json:"inputs"`  // readonly
	Outputs       []*Output  `json:"outputs"` // readonly
}

// MarshalJSON helper to marshal unix time
func (b *Bucket) MarshalJSON() ([]byte, error) {
	type Alias Bucket
	return json.Marshal(&struct {
		CreatedAt int64 `json:"created_at"`
		UpdatedAt int64 `json:"updated_at"`
		*Alias
	}{
		CreatedAt: b.CreatedAt.Unix(),
		UpdatedAt: b.UpdatedAt.Unix(),
		Alias:     (*Alias)(b),
	})
}

// UnmarshalJSON helper to unmarshal unix time or RFC3339 string
func (b *Bucket) UnmarshalJSON(data []byte) error {
	type Alias Bucket
	aux := &struct {
		CreatedAt json.RawMessage `json:"created_at"`
		UpdatedAt json.RawMessage `json:"updated_at"`
		*Alias
	}{
		Alias: (*Alias)(b),
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	var err error
	b.CreatedAt, err = parseTime(aux.CreatedAt)
	if err != nil {
		return err
	}
	b.UpdatedAt, err = parseTime(aux.UpdatedAt)
	if err != nil {
		return err
	}
	return nil
}

// BucketAuth specifies authentication method for incoming requests to the bucket's inputs
type BucketAuth struct {
	Type     AuthType `json:"type"`
	Username string   `json:"username,omitempty"`
	Password string   `json:"password,omitempty"`
	Token    string   `json:"token,omitempty"`
}

// BucketCreateOptions create opts
type BucketCreateOptions struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// BucketDeleteOptions are used to delete bucket
type BucketDeleteOptions struct {
	Ref   string `json:"ref"`
	Force bool   `json:"force"`
}

// BucketListOptions - TODO
type BucketListOptions struct{}

// ListBuckets lists buckets for an account
func (api *API) ListBuckets(options *BucketListOptions) ([]*Bucket, error) {
	resp, err := api.makeRequest(http.MethodGet, "/buckets", nil)
	if err != nil {
		return nil, errors.Wrap(err, errMakeRequestError)
	}

	var buckets []*Bucket
	err = json.Unmarshal(resp, &buckets)
	if err != nil {
		return nil, errors.Wrap(err, errUnmarshalError)
	}

	return buckets, nil
}

// GetBucket gets specific bucket
func (api *API) GetBucket(ref string) (*Bucket, error) {

	ref, err := api.ensureBucketID(ref)
	if err != nil {
		return nil, err
	}

	resp, err := api.makeRequest("GET", "/buckets/"+ref, nil)
	if err != nil {
		return nil, err
	}

	var result Bucket
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// CreateBucket creates a Bucket and returns the newly object.
func (api *API) CreateBucket(options *BucketCreateOptions) (*Bucket, error) {
	resp, err := api.makeRequest("POST", "/buckets", options)
	if err != nil {
		return nil, err
	}

	var bucket Bucket
	if err := json.Unmarshal(resp, &bucket); err != nil {
		return nil, err
	}
	return &bucket, nil
}

// UpdateBucket updates a Bucket on the server and returns the updated object.
func (api *API) UpdateBucket(options *Bucket) (*Bucket, error) {
	bucketID, err := api.ensureBucketID(options.ID)
	if err != nil {
		return nil, err
	}
	options.ID = bucketID

	resp, err := api.makeRequest("PUT", "/buckets/"+options.ID, options)
	if err != nil {
		return nil, err
	}

	var bucket Bucket
	if err := json.Unmarshal(resp, &bucket); err != nil {
		return nil, err
	}
	return &bucket, nil
}

// DeleteBucket removes a Bucket by its reference.
func (api *API) DeleteBucket(options *BucketDeleteOptions) error {

	bucketID, err := api.ensureBucketID(options.Ref)
	if err != nil {
		return err
	}

	_, err = api.makeRequest("DELETE", "/buckets/"+bucketID, nil)
	if err != nil {
		return err
	}

	return nil
}

// ensureBucketID - takes name/id and always returns ID (when it not fails)
func (api *API) ensureBucketID(ref string) (string, error) {
	if !IsUUID(ref) {
		id, err := api.bucketIDFromName(ref)
		if err != nil {
			return "", err
		}
		return id, nil
	}
	return ref, nil
}

func (api *API) bucketIDFromName(name string) (id string, err error) {
	buckets, err := api.ListBuckets(&BucketListOptions{})
	if err != nil {
		return
	}
	for _, b := range buckets {
		if b.Name == name {
			return b.ID, nil
		}
	}
	return "", fmt.Errorf("no such bucket '%s'", name)
}
