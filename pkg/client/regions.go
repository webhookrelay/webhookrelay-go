package client

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/pkg/errors"
)

// Region is a server entry that accepts tunnel connections and acts
// as a hub
type Region struct {
	ID        string    `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Name      string    `json:"name"`
	// for default region it's just [xx].webrelay.io
	// but for Australia region it could be [xx].au.webrelay.io or even
	// a completely different, non webrelay domain. This way in theory we could allow
	// self-hosted but managed regions
	DomainSuffix string `json:"domain_suffix"`

	// ServerAddress is a tunneling server HOSTNAME:PORT address
	ServerAddress string `json:"server_address"`
}

// MarshalJSON helper to marshal unix time
func (r *Region) MarshalJSON() ([]byte, error) {
	type Alias Region
	return json.Marshal(&struct {
		CreatedAt int64 `json:"created_at"`
		UpdatedAt int64 `json:"updated_at"`
		*Alias
	}{
		CreatedAt: r.CreatedAt.Unix(),
		UpdatedAt: r.UpdatedAt.Unix(),
		Alias:     (*Alias)(r),
	})
}

// UnmarshalJSON helper to unmarshal unix time
func (r *Region) UnmarshalJSON(data []byte) error {
	type Alias Region
	aux := &struct {
		CreatedAt int64 `json:"created_at"`
		UpdatedAt int64 `json:"updated_at"`
		*Alias
	}{
		Alias: (*Alias)(r),
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	r.CreatedAt = time.Unix(aux.CreatedAt, 0)
	r.UpdatedAt = time.Unix(aux.UpdatedAt, 0)
	return nil
}

// RegionListOptions - region list options
type RegionListOptions struct{}

// ListRegions lists available regions
func (api *API) ListRegions(options *RegionListOptions) ([]*Region, error) {
	resp, err := api.makeRequest(http.MethodGet, "/regions", nil)
	if err != nil {
		return nil, errors.Wrap(err, errMakeRequestError)
	}

	var result []*Region
	err = json.Unmarshal(resp, &result)
	if err != nil {
		return nil, errors.Wrap(err, errUnmarshalError)
	}

	return result, nil
}
