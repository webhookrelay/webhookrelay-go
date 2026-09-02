package webhookrelay

import (
	"encoding/json"
	"fmt"
)

// HeaderValues is a set of HTTP headers attached to a resource — the headers a
// cron injects into the request it fires, or those an input or output adds in
// flight.
//
// It exists because the API stores these as an opaque JSON object and returns
// whatever was written to it, so both shapes occur on real accounts:
//
//	{"Authorization": "Bearer ..."}      // written by the dashboard
//	{"Authorization": ["Bearer ..."]}    // written by this client
//
// A plain map[string][]string decodes only the second, and fails the whole
// request on the first — an account with one dashboard-created cron carrying an
// auth header could not list its crons at all.
//
// Decoding accepts either form. Encoding always emits the list form, which the
// API accepts, so a value read and written back is unchanged in meaning.
type HeaderValues map[string][]string

// UnmarshalJSON accepts a scalar or a list for each header value.
func (h *HeaderValues) UnmarshalJSON(data []byte) error {
	if len(data) == 0 || string(data) == "null" {
		*h = nil
		return nil
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("headers: %w", err)
	}

	out := make(HeaderValues, len(raw))
	for name, value := range raw {
		var list []string
		if err := json.Unmarshal(value, &list); err == nil {
			out[name] = list
			continue
		}
		var single string
		if err := json.Unmarshal(value, &single); err != nil {
			// Naming the header matters: the alternative is a decode error
			// against a list of a hundred crons with no clue which one.
			return fmt.Errorf("headers: %q must be a string or a list of strings, got %s",
				name, string(value))
		}
		out[name] = []string{single}
	}
	*h = out
	return nil
}

// Get returns the first value for name, or "" when it is not set. Callers
// almost always want one value, and indexing the map directly to reach [0]
// panics on a header that was present but empty.
func (h HeaderValues) Get(name string) string {
	if values := h[name]; len(values) > 0 {
		return values[0]
	}
	return ""
}
