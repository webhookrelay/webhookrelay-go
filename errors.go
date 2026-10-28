package webhookrelay

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"
)

// Errors
var (
	ErrEmptyCredentials = errors.New("invalid credentials: key & secret must not be empty")
)

// Error messages
var (
	errMakeRequestError = "error from makeRequest"
	errUnmarshalError   = "error while unmarshalling the JSON response"
)

// APIError is returned when the API answers with a non-2xx status. Find it in
// a returned error with errors.As to branch on the status code:
//
//	var apiErr *webhookrelay.APIError
//	if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusBadRequest {
//		// fix the request
//	}
//
// Transport failures (DNS, connection refused, timeouts) are not APIErrors.
type APIError struct {
	// StatusCode is the HTTP status the API answered with.
	StatusCode int
	// Message is the "error" field of a {"error":"..."} response body, empty
	// when the body has no such field.
	Message string
	// Body is the raw response body, for example {"error":"..."} from the
	// outbound API.
	Body string
	// RetryAfter is the delay the server asked for in its Retry-After header
	// (usually on 429 and 503), zero when absent.
	RetryAfter time.Duration
}

// Error keeps the text of earlier releases, which callers match on: fixed
// "HTTP status <code>: ..." strings, and the raw body for 400s.
func (e *APIError) Error() string {
	switch e.StatusCode {
	case http.StatusUnauthorized:
		return fmt.Sprintf("HTTP status %d: invalid credentials", e.StatusCode)
	case http.StatusForbidden:
		return fmt.Sprintf("HTTP status %d: insufficient permissions", e.StatusCode)
	case http.StatusPreconditionFailed:
		return fmt.Sprintf("HTTP status %d: precondition failed", e.StatusCode)
	case http.StatusPaymentRequired:
		return fmt.Sprintf("HTTP status %d: feature not available for your subscription", e.StatusCode)
	case http.StatusServiceUnavailable, http.StatusBadGateway, http.StatusGatewayTimeout, 522, 523, 524:
		return fmt.Sprintf("HTTP status %d: service failure", e.StatusCode)
	case http.StatusBadRequest:
		if len(e.Body) > 0 {
			return e.Body
		}
		return fmt.Sprintf("HTTP status %d: bad request", e.StatusCode)
	default:
		return fmt.Sprintf("HTTP status %d: content %q", e.StatusCode, e.Body)
	}
}

// newAPIError builds the error for a non-2xx response.
func newAPIError(status int, header http.Header, body []byte) *APIError {
	apiErr := &APIError{StatusCode: status, Body: string(body)}
	if wait, ok := parseRetryAfter(header.Get("Retry-After"), time.Now()); ok {
		apiErr.RetryAfter = wait
	}
	var payload struct {
		Error string `json:"error"`
	}
	if json.Unmarshal(body, &payload) == nil {
		apiErr.Message = payload.Error
	}
	return apiErr
}

// IsNotFound reports whether err is an APIError with status 404.
func IsNotFound(err error) bool { return hasStatus(err, http.StatusNotFound) }

// IsBadRequest reports whether err is an APIError with status 400.
func IsBadRequest(err error) bool { return hasStatus(err, http.StatusBadRequest) }

// IsConflict reports whether err is an APIError with status 409.
func IsConflict(err error) bool { return hasStatus(err, http.StatusConflict) }

// IsRateLimited reports whether err is an APIError with status 429.
func IsRateLimited(err error) bool { return hasStatus(err, http.StatusTooManyRequests) }

func hasStatus(err error, status int) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr) && apiErr.StatusCode == status
}
