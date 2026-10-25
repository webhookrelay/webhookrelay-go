package webhookrelay

import "errors"

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
type APIError struct {
	// StatusCode is the HTTP status the API answered with.
	StatusCode int
	// Body is the raw response body, for example {"error":"..."} from the
	// outbound API.
	Body string

	message string
}

func (e *APIError) Error() string {
	return e.message
}
