package webhookrelay

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/pkg/errors"
)

// Domain is a domain reservation
type Domain struct {
	ID        string    `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// Domain can be any 3rd party domain such as
	// user.example.com
	Domain string `json:"domain"`
}

// MarshalJSON helper to marshal unix time
func (d *Domain) MarshalJSON() ([]byte, error) {
	type Alias Domain
	return json.Marshal(&struct {
		CreatedAt int64 `json:"created_at"`
		UpdatedAt int64 `json:"updated_at"`
		*Alias
	}{
		CreatedAt: d.CreatedAt.Unix(),
		UpdatedAt: d.UpdatedAt.Unix(),
		Alias:     (*Alias)(d),
	})
}

// UnmarshalJSON helper to unmarshal unix time or RFC3339 string
func (d *Domain) UnmarshalJSON(data []byte) error {
	type Alias Domain
	aux := &struct {
		CreatedAt json.RawMessage `json:"created_at"`
		UpdatedAt json.RawMessage `json:"updated_at"`
		*Alias
	}{
		Alias: (*Alias)(d),
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	var err error
	d.CreatedAt, err = parseTime(aux.CreatedAt)
	if err != nil {
		return err
	}
	d.UpdatedAt, err = parseTime(aux.UpdatedAt)
	if err != nil {
		return err
	}
	return nil
}

// DomainDeleteOptions are used to delete domain reservation
type DomainDeleteOptions struct {
	Ref string `json:"ref"`
}

// DomainListOptions - TODO
type DomainListOptions struct{}

// ListDomainReservations lists domain reservations for an account
func (api *API) ListDomainReservations(options *DomainListOptions) ([]*Domain, error) {
	resp, err := api.makeRequest(http.MethodGet, "/domains", nil)
	if err != nil {
		return nil, errors.Wrap(err, errMakeRequestError)
	}

	var domains []*Domain
	err = json.Unmarshal(resp, &domains)
	if err != nil {
		return nil, errors.Wrap(err, errUnmarshalError)
	}

	return domains, nil
}

// ReserveDomain - reserve domain
func (api *API) ReserveDomain(options *Domain) (*Domain, error) {
	resp, err := api.makeRequest(http.MethodPost, "/domains", options)
	if err != nil {
		return nil, err
	}

	var domainReservation Domain
	if err := json.Unmarshal(resp, &domainReservation); err != nil {
		return nil, err
	}
	return &domainReservation, nil
}

// DeleteDomainReservation deletes domain reservation. It can only be removed
// once no Input or Tunnel is using it.
func (api *API) DeleteDomainReservation(options *DomainDeleteOptions) error {

	if !IsUUID(options.Ref) {
		var err error
		options.Ref, err = api.domainIDFromName(options.Ref)
		if err != nil {
			return err
		}
	}

	_, err := api.makeRequest(http.MethodDelete, "/domains/"+options.Ref, nil)
	if err != nil {
		return err
	}

	return nil
}

func (api *API) domainIDFromName(domainName string) (id string, err error) {
	domains, err := api.ListDomainReservations(&DomainListOptions{})
	if err != nil {
		return
	}
	for _, b := range domains {
		if b.Domain == domainName {
			return b.ID, nil
		}
	}
	return "", fmt.Errorf("no such domain '%s'", domainName)
}
