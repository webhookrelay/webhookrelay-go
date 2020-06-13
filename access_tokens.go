package webhookrelay

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/pkg/errors"
)

// AccessToken - auth tokens, can be created for the agents
type AccessToken struct {
	ID          string            `json:"id"`         // read-only
	CreatedAt   time.Time         `json:"created_at"` // read-only
	UpdatedAt   time.Time         `json:"updated_at"` // read-only
	LastLogin   string            `json:"last_login"` // read-only
	Description string            `json:"description"`
	Scopes      AccessTokenScopes `json:"scopes"`
	// APIAccess allows to enable/disabled API access. Tokens that have disabled
	// access can be used to subscribe to webhooks or tunnel connections.
	// Defaults to "enabled"
	APIAccess AccessTokenAPIAccess `json:"api_access"`
	Active    bool                 `json:"active"`
}

// MarshalJSON helper to marshal unix time
func (t *AccessToken) MarshalJSON() ([]byte, error) {
	type Alias AccessToken
	return json.Marshal(&struct {
		CreatedAt int64 `json:"created_at"`
		UpdatedAt int64 `json:"updated_at"`
		*Alias
	}{
		CreatedAt: t.CreatedAt.Unix(),
		UpdatedAt: t.UpdatedAt.Unix(),
		Alias:     (*Alias)(t),
	})
}

// UnmarshalJSON helper to unmarshal unix time
func (t *AccessToken) UnmarshalJSON(data []byte) error {
	type Alias AccessToken
	aux := &struct {
		CreatedAt int64 `json:"created_at"`
		UpdatedAt int64 `json:"updated_at"`
		*Alias
	}{
		Alias: (*Alias)(t),
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	t.CreatedAt = time.Unix(aux.CreatedAt, 0)
	t.UpdatedAt = time.Unix(aux.UpdatedAt, 0)
	return nil
}

// AccessTokenCreateResponse - response when creating a token
type AccessTokenCreateResponse struct {
	Key    string `json:"key"`
	Secret string `json:"secret"`
}

// AccessTokenAPIAccess - enables/disables API access for the token
type AccessTokenAPIAccess string

// Available API access token status
const (
	AccessTokenAPIAccessEnabled  AccessTokenAPIAccess = "enabled"
	AccessTokenAPIAccessDisabled AccessTokenAPIAccess = "disabled"
)

// AccessTokenCreateOptions - used to create an access token
type AccessTokenCreateOptions struct {
	Description string               `json:"description"`
	Scopes      AccessTokenScopes    `json:"scopes"`
	APIAccess   AccessTokenAPIAccess `json:"api_access"`
}

// AccessTokenScopes define optional limits for tokens
type AccessTokenScopes struct {
	Tunnels []string `json:"tunnels"`
	Buckets []string `json:"buckets"`
}

// AccessTokenListOptions - TODO
type AccessTokenListOptions struct{}

// AccessTokenDeleteOptions used to delete access token
type AccessTokenDeleteOptions struct {
	ID string `json:"id"` // ID/Key
}

// ListAccessTokens lists access tokens for an account
func (api *API) ListAccessTokens(options *AccessTokenListOptions) ([]*AccessToken, error) {
	resp, err := api.makeRequest(http.MethodGet, "/tokens", nil)
	if err != nil {
		return nil, errors.Wrap(err, errMakeRequestError)
	}

	var data []*AccessToken
	err = json.Unmarshal(resp, &data)
	if err != nil {
		return nil, errors.Wrap(err, errUnmarshalError)
	}

	return data, nil
}

// CreateAccessToken - create new access token. Returned Key and Secret pair
// should be saved on user's side. Server has already hashed the secret so it can't
// be recovered. If the secret is lost, just create a new access token.
func (api *API) CreateAccessToken(options *AccessTokenCreateOptions) (*AccessTokenCreateResponse, error) {
	resp, err := api.makeRequest(http.MethodPost, "/tokens", options)
	if err != nil {
		return nil, err
	}

	var keyAndSecretPair AccessTokenCreateResponse
	if err := json.Unmarshal(resp, &keyAndSecretPair); err != nil {
		return nil, err
	}
	return &keyAndSecretPair, nil
}

// DeleteAccessToken deletes access token
func (api *API) DeleteAccessToken(options *AccessTokenDeleteOptions) error {

	if !IsUUID(options.ID) {
		return fmt.Errorf("invalid access token ID '%s'", options.ID)
	}

	_, err := api.makeRequest(http.MethodDelete, "/tokens/"+options.ID, nil)
	return err
}

// UpdateAccessToken updates access token scopes, description and enabled/disable API access
func (api *API) UpdateAccessToken(options *AccessToken) (*AccessToken, error) {
	if !IsUUID(options.ID) {
		return nil, fmt.Errorf("invalid access token ID '%s'", options.ID)
	}

	resp, err := api.makeRequest(http.MethodPut, "/tokens/"+options.ID, options)
	if err != nil {
		return nil, err
	}

	var result AccessToken
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, err
	}
	return &result, nil
}
