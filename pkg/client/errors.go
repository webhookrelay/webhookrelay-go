package client

import "errors"

// Error messages
var (
	ErrEmptyCredentials = errors.New("invalid credentials: key & secret must not be empty")
)
