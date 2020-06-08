// generated by jsonenums -type=AuthType; DO NOT EDIT

package client

import (
	"encoding/json"
	"fmt"
)

var (
	_AuthTypeNameToValue = map[string]AuthType{
		"AuthTypeNone":  AuthTypeNone,
		"AuthTypeBasic": AuthTypeBasic,
		"AuthTypeToken": AuthTypeToken,
	}

	_AuthTypeValueToName = map[AuthType]string{
		AuthTypeNone:  "AuthTypeNone",
		AuthTypeBasic: "AuthTypeBasic",
		AuthTypeToken: "AuthTypeToken",
	}
)

func init() {
	var v AuthType
	if _, ok := interface{}(v).(fmt.Stringer); ok {
		_AuthTypeNameToValue = map[string]AuthType{
			interface{}(AuthTypeNone).(fmt.Stringer).String():  AuthTypeNone,
			interface{}(AuthTypeBasic).(fmt.Stringer).String(): AuthTypeBasic,
			interface{}(AuthTypeToken).(fmt.Stringer).String(): AuthTypeToken,
		}
	}
}

// MarshalJSON is generated so AuthType satisfies json.Marshaler.
func (r AuthType) MarshalJSON() ([]byte, error) {
	if s, ok := interface{}(r).(fmt.Stringer); ok {
		return json.Marshal(s.String())
	}
	s, ok := _AuthTypeValueToName[r]
	if !ok {
		return nil, fmt.Errorf("invalid AuthType: %d", r)
	}
	return json.Marshal(s)
}

// UnmarshalJSON is generated so AuthType satisfies json.Unmarshaler.
func (r *AuthType) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return fmt.Errorf("AuthType should be a string, got %s", data)
	}
	v, ok := _AuthTypeNameToValue[s]
	if !ok {
		return fmt.Errorf("invalid AuthType %q", s)
	}
	*r = v
	return nil
}