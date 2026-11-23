package webhookrelay

import (
	"net/http"
	"time"
)

// Outbound environments separate, for example, test traffic from live: each
// environment owns its consumers, event types, endpoints, messages, recovery
// tasks, imports and portal sessions. The "default" environment always exists
// and holds everything created without one. Quotas are per account across
// environments.
//
// A client acts in one environment, set with WithOutboundEnvironment or
// derived with API.OutboundEnvironment. A token bound to an environment
// always acts in it, and a request naming another one is refused with 403.

// OutboundEnvironmentHeader selects the environment of an outbound request.
const OutboundEnvironmentHeader = "X-Outbound-Environment"

// OutboundDefaultEnvironment always exists and cannot be created or deleted.
const OutboundDefaultEnvironment = "default"

// OutboundEnvironment is a producer-named environment such as "test".
type OutboundEnvironment struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	// Default is true for the default environment.
	Default   bool      `json:"default"`
	CreatedAt time.Time `json:"created_at"`
}

// OutboundEnvironmentOptions creates an environment.
type OutboundEnvironmentOptions struct {
	// Name is 1-32 lowercase letters, digits, "-" or "_", starting with a
	// letter or digit.
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// WithOutboundEnvironment makes every outbound call act in the named
// environment (sent as the X-Outbound-Environment header). Empty means the
// default environment, or the one the token is bound to. Calls in an
// environment that does not exist fail with 404.
func WithOutboundEnvironment(name string) Option {
	return func(api *API) error {
		api.outboundEnvironment = name
		return nil
	}
}

// OutboundEnvironment returns a shallow copy of the client whose outbound
// calls act in the named environment. Like WithContext it shares the HTTP
// client, rate limiter and configuration, so it is cheap to create:
//
//	live := api.OutboundEnvironment("live")
//	_, err := live.PublishOutboundMessage(...)
func (api *API) OutboundEnvironment(name string) *API {
	clone := *api
	clone.outboundEnvironment = name
	return &clone
}

// OutboundEnvironmentName is the environment the client's outbound calls act
// in; empty means the default one.
func (api *API) OutboundEnvironmentName() string {
	return api.outboundEnvironment
}

// ListOutboundEnvironments returns the account's environments, the default
// one included.
func (api *API) ListOutboundEnvironments() ([]*OutboundEnvironment, error) {
	var environments []*OutboundEnvironment
	err := api.outbound(http.MethodGet, outboundPath("environments"), nil, &environments, nil)
	return environments, err
}

// CreateOutboundEnvironment creates an environment. An account can have 20
// besides the default one; an existing name is refused with 409.
func (api *API) CreateOutboundEnvironment(options *OutboundEnvironmentOptions) (*OutboundEnvironment, error) {
	var environment OutboundEnvironment
	if err := api.outbound(http.MethodPost, outboundPath("environments"), options, &environment, nil); err != nil {
		return nil, err
	}
	return &environment, nil
}

// DeleteOutboundEnvironment deletes an environment. It is refused with 409
// while the environment still has consumers or event types, and with 400 for
// the default one.
func (api *API) DeleteOutboundEnvironment(name string) error {
	return api.outbound(http.MethodDelete, outboundPath("environments", name), nil, nil, nil)
}
