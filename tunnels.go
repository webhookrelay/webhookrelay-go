//go:generate jsonenums -type=TunnelMode
//go:generate jsonenums -type=TunnelProto
package webhookrelay

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/pkg/errors"
)

// TunnelMode - tunnel mode
type TunnelMode int

// available tunnel modes
const (
	TunnelModeInactive TunnelMode = iota
	TunnelModeActive
)

// ParseTunnelMode - parses tunnel mode string
func ParseTunnelMode(mode string) TunnelMode {
	switch mode {
	case "inactive":
		return TunnelModeInactive
	case "active":
		return TunnelModeActive
	default:
		// defaulting to active
		return TunnelModeActive
	}
}

func (s TunnelMode) String() string {
	switch s {
	case TunnelModeInactive:
		return "inactive"
	case TunnelModeActive:
		return "active"
	default:
		return "unknown"
	}
}

// TunnelAuth - optional auth for tunnels
type TunnelAuth struct {
	Type AuthType `json:"type"`

	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
	Token    string `json:"token,omitempty"`
}

// Tunnel is a type to store bidirectional tunnel data
// these tunnels are different from webhook buckets in a way
// that they provide responses to whatever calls them
type Tunnel struct {
	ID           string       `json:"id"`
	CreatedAt    time.Time    `json:"created_at"`
	UpdatedAt    time.Time    `json:"updated_at"`
	Name         string       `json:"name"` // tunnel name
	Group        string       `json:"group"`
	Region       string       `json:"region"`
	Destination  string       `json:"destination"` // destination host, defaults to 127.0.0.1:8000
	Host         string       `json:"host"`
	Addr         string       `json:"addr"`
	Mode         TunnelMode   `json:"mode"`
	Protocol     string       `json:"protocol"` // tunnel protocol - http/tcp
	Crypto       string       `json:"crypto"`
	AccountID    string       `json:"account_id"`
	Description  string       `json:"description"`
	Auth         TunnelAuth   `json:"auth"`
	Features     Features     `json:"features"`
	IngressRules IngressRules `json:"ingress_rules"`
}

// Features - optional tunnel features to enable different functionality
type Features struct {
	RewriteHostHeader string `json:"rewrite_host_header"`
}

// GetURL helper
func (t *Tunnel) GetURL() string {
	switch t.Crypto {
	case CryptoOff:
		return "http://" + t.Host
	default:
		return "https://" + t.Host
	}
}

// IngressRules - ingress defines custom routing configuration based on paths
type IngressRules struct {
	Rules []*IngressRule `json:"rules"`
}

// Endpoint - is an address where request should be routed
type Endpoint struct {
	Address string `json:"address"`
}

// IngressRule is used by the ingress controller to route to multiple targets
type IngressRule struct {
	// Name is an option identifier for the ingress rule, it usually is a service name
	// if used with webrelay-ingress
	Name string `json:"name"`

	// Path is an extended POSIX regex as defined by IEEE Std 1003.1,
	// (i.e this follows the egrep/unix syntax, not the perl syntax)
	// matched against the path of an incoming request. Currently it can
	// contain characters disallowed from the conventional "path"
	// part of a URL as defined by RFC 3986. Paths must begin with
	// a '/'. If unspecified, the path defaults to a catch all sending
	// traffic to the backend.
	// +optional
	Path string `json:"path"`

	// Endpoints
	Endpoints []*Endpoint `json:"endpoints"`
}

// tunnel crypto types
const (
	CryptoOff            = "off"
	CryptoFlexible       = "flexible"
	CryptoFull           = "full"
	CryptoFullStrict     = "full-strict"
	CryptoTLSPassThrough = "tls-pass-through"
)

// MarshalJSON marshal to unix time
func (t *Tunnel) MarshalJSON() ([]byte, error) {
	type Alias Tunnel
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

// UnmarshalJSON unamrshal unix time
func (t *Tunnel) UnmarshalJSON(data []byte) error {
	type Alias Tunnel
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

// TunnelListOptions - list tunnels options
type TunnelListOptions struct{}

// ListTunnels lists tunnels for an account
func (api *API) ListTunnels(options *TunnelListOptions) ([]*Tunnel, error) {
	resp, err := api.makeRequest(http.MethodGet, "/tunnels", nil)
	if err != nil {
		return nil, errors.Wrap(err, errMakeRequestError)
	}

	var tunnels []*Tunnel
	err = json.Unmarshal(resp, &tunnels)
	if err != nil {
		return nil, errors.Wrap(err, errUnmarshalError)
	}

	return tunnels, nil
}

// GetTunnel gets tunnel by ID, name or hostname
func (api *API) GetTunnel(ref string) (*Tunnel, error) {

	ref, err := api.ensureTunnelID(ref)
	if err != nil {
		return nil, err
	}

	resp, err := api.makeRequest(http.MethodGet, "/tunnels/"+ref, nil)
	if err != nil {
		return nil, err
	}

	var result Tunnel
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// CreateTunnel creates new tunnel
func (api *API) CreateTunnel(options *Tunnel) (*Tunnel, error) {
	resp, err := api.makeRequest(http.MethodPost, "/tunnels", options)
	if err != nil {
		return nil, err
	}

	var result Tunnel
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// UpdateTunnel updates existing tunnel
func (api *API) UpdateTunnel(options *Tunnel) (*Tunnel, error) {
	tunnelID, err := api.ensureTunnelID(options.ID)
	if err != nil {
		return nil, err
	}
	options.ID = tunnelID

	resp, err := api.makeRequest(http.MethodPut, "/tunnels/"+options.ID, options)
	if err != nil {
		return nil, err
	}

	var result Tunnel
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// TunnelDeleteOptions delete options
type TunnelDeleteOptions struct {
	ID   string
	Name string
}

// DeleteTunnel delete tunnel by ID or name
func (api *API) DeleteTunnel(options *TunnelDeleteOptions) error {

	if options.ID == "" && options.Name == "" {
		return fmt.Errorf("name or ID must be supplied")
	}

	var identifier string
	if options.ID != "" {
		identifier = options.ID
	} else {
		identifier = options.Name
	}

	tunnelID, err := api.ensureTunnelID(identifier)
	if err != nil {
		return err
	}

	_, err = api.makeRequest("DELETE", "/tunnels/"+tunnelID, nil)
	if err != nil {
		return err
	}

	return nil
}

func (api *API) ensureTunnelID(ref string) (string, error) {
	if !IsUUID(ref) {
		id, err := api.tunnelIDFromName(ref)
		if err != nil {
			return "", err
		}
		return id, nil
	}
	return ref, nil
}

func (api *API) tunnelIDFromName(ref string) (id string, err error) {
	tunnels, err := api.ListTunnels(&TunnelListOptions{})
	if err != nil {
		return
	}
	for _, t := range tunnels {
		if t.Name == ref || t.Host == ref {
			return t.ID, nil
		}
	}
	return "", fmt.Errorf("no such tunnel '%s'", ref)
}
