package client

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/pkg/errors"
)

// Bucket - bucket is required for webhook inputs and outputs. There
// should probably be one Input per Bucket to make it easy to manage.
// Buckets control policies such as retries, manipulation, logs, rate limitting
type Bucket struct {
	ID          string     `json:"id"`         // readonly
	CreatedAt   time.Time  `json:"created_at"` // readonly
	UpdatedAt   time.Time  `json:"updated_at"` // readonly
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Stream      bool       `json:"stream"`
	Ephemeral   bool       `json:"ephemeral"`
	Auth        BucketAuth `json:"auth"`
	Inputs      []Input    `json:"inputs"`  // readonly
	Outputs     []Output   `json:"outputs"` // readonly
}

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

func (b *Bucket) UnmarshalJSON(data []byte) error {
	type Alias Bucket
	aux := &struct {
		CreatedAt int64 `json:"created_at"`
		UpdatedAt int64 `json:"updated_at"`
		*Alias
	}{
		Alias: (*Alias)(b),
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	b.CreatedAt = time.Unix(aux.CreatedAt, 0)
	b.UpdatedAt = time.Unix(aux.UpdatedAt, 0)
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

	// Context can be set with a timeout or can be used to cancel a request.
	Context context.Context `json:"-"`
}

// BucketUpdateOptions - bucket update opts
type BucketUpdateOptions struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`

	// Context can be set with a timeout or can be used to cancel a request.
	Context context.Context `json:"-"`
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