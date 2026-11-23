package webhookrelay

import (
	"fmt"
	"net/http"
	"time"
)

// The consumer portal lets one of your customers manage their own endpoints
// and read their deliveries, embedded in your app or opened as a page. Mint a
// link per page load, server-side: it grants access to that consumer.

// Portal capabilities.
const (
	// OutboundPortalManage reads, changes endpoints, sends tests and
	// recovers deliveries.
	OutboundPortalManage = "manage"
	// OutboundPortalRead only reads.
	OutboundPortalRead = "read"
)

// OutboundPortalSessionOptions mints a portal link. Every field is optional.
type OutboundPortalSessionOptions struct {
	// Capabilities is OutboundPortalManage (the default) or OutboundPortalRead.
	Capabilities string
	// TTL is how long the session lasts once opened: 0 (12 hours) or 1 hour
	// to 7 days in whole seconds. CreateOutboundPortalSession refuses any
	// other value instead of rounding it.
	TTL time.Duration
	// Next opens the portal on one page: "endpoints/<id>", "endpoints/new",
	// "event-types" or "messages/<id>".
	Next string
	// SessionID is your own tag, to end the session later with
	// ExpireOutboundPortalSessions.
	SessionID string
}

// OutboundPortalLink is a consumer portal URL.
type OutboundPortalLink struct {
	// URL carries a one-time token in its fragment: never log it. Embed it in
	// an iframe (the page's origin must be in the portal settings'
	// allowed_origins) or open it.
	URL string `json:"url"`
	// ExpiresAt is when the session ends.
	ExpiresAt time.Time `json:"expires_at"`
	// TokenExpiresAt is when the link stops working if it was never opened.
	TokenExpiresAt time.Time `json:"token_expires_at"`
}

type outboundPortalSessionBody struct {
	Capabilities string `json:"capabilities,omitempty"`
	TTL          int    `json:"ttl,omitempty"`
	Next         string `json:"next,omitempty"`
	SessionID    string `json:"session_id,omitempty"`
}

// CreateOutboundPortalSession mints a portal link for a consumer, in the
// client's environment. options may be nil.
func (api *API) CreateOutboundPortalSession(consumer string, options *OutboundPortalSessionOptions) (*OutboundPortalLink, error) {
	if options == nil {
		options = &OutboundPortalSessionOptions{}
	}
	if err := validatePortalSessionTTL(options.TTL); err != nil {
		return nil, err
	}
	body := outboundPortalSessionBody{
		Capabilities: options.Capabilities,
		TTL:          int(options.TTL / time.Second),
		Next:         options.Next,
		SessionID:    options.SessionID,
	}
	var link OutboundPortalLink
	if err := api.outbound(http.MethodPost, outboundPath("consumers", consumer, "portal-sessions"), body, &link, nil); err != nil {
		return nil, err
	}
	return &link, nil
}

// ExpireOutboundPortalSessions ends the consumer's open portal sessions, or
// only those tagged sessionID when it is not empty, for example when your
// user signs out. It returns how many were ended.
func (api *API) ExpireOutboundPortalSessions(consumer, sessionID string) (int64, error) {
	body := struct {
		SessionID string `json:"session_id,omitempty"`
	}{SessionID: sessionID}
	var out struct {
		Ended int64 `json:"ended"`
	}
	if err := api.outbound(http.MethodPost, outboundPath("consumers", consumer, "portal-sessions", "expire"), body, &out, nil); err != nil {
		return 0, err
	}
	return out.Ended, nil
}

// Portal session TTL bounds the API accepts.
const (
	minOutboundPortalSessionTTL = time.Hour
	maxOutboundPortalSessionTTL = 7 * 24 * time.Hour
)

// validatePortalSessionTTL refuses a TTL the API would reject or that would
// lose its fraction of a second when sent.
func validatePortalSessionTTL(ttl time.Duration) error {
	if ttl == 0 {
		return nil
	}
	if ttl < minOutboundPortalSessionTTL || ttl > maxOutboundPortalSessionTTL {
		return fmt.Errorf("portal session TTL %s is outside 1h to 168h", ttl)
	}
	if ttl%time.Second != 0 {
		return fmt.Errorf("portal session TTL %s is not a whole number of seconds", ttl)
	}
	return nil
}
