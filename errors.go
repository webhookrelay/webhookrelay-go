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
