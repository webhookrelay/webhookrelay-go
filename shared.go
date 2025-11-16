//go:generate jsonenums -type=AuthType
package webhookrelay

import (
	"encoding/json"
	"fmt"
	"time"
)

// AuthType is a tunnel authentication type
type AuthType int

// Available tunnel authentication modes
const (
	AuthTypeNone AuthType = iota
	AuthTypeBasic
	AuthTypeToken
)

func (s AuthType) String() string {
	switch s {
	case AuthTypeBasic:
		return "basic"
	case AuthTypeToken:
		return "token"
	default:
		return "none"
	}
}

func parseTime(data json.RawMessage) (time.Time, error) {
	if len(data) == 0 {
		return time.Time{}, nil
	}

	var unixTime int64
	if err := json.Unmarshal(data, &unixTime); err == nil {
		return time.Unix(unixTime, 0), nil
	}

	var timeStr string
	if err := json.Unmarshal(data, &timeStr); err == nil {
		return time.Parse(time.RFC3339, timeStr)
	}

	var t time.Time
	if err := json.Unmarshal(data, &t); err == nil {
		return t, nil
	}

	return time.Time{}, fmt.Errorf("unable to parse time: %s", string(data))
}
