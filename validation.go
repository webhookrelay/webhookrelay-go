package webhookrelay

import (
	"errors"
	"regexp"
)

const (
	// IDFormat are the characters allowed to represent an ID.
	IDFormat = `[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`

	// NameFormat are the characters allowed to represent a name.
	NameFormat = `[a-zA-Z0-9][a-zA-Z0-9~_.-]+`
)

var (
	// IDPattern is a regular expression to validate a unique id against the
	// collection of restricted characters.
	IDPattern = regexp.MustCompile(`^` + IDFormat + `$`)

	// NamePattern is a regular expression to validate names against the
	// collection of restricted characters.
	NamePattern = regexp.MustCompile(`^` + NameFormat + `$`)

	// ErrNoRef returned when ref is incorrect
	ErrNoRef = errors.New("no ref provided or incorrect format")
)

// IsUUID returns true if the string input is a valid UUID string.
func IsUUID(s string) bool {
	return IDPattern.MatchString(s)
}

// IsName returns true if the string input is a valid Name string.
func IsName(s string) bool {
	return NamePattern.MatchString(s)
}
