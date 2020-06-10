// generated by jsonenums -type=TunnelMode; DO NOT EDIT

package webhookrelay

import (
	"encoding/json"
	"fmt"
)

var (
	_TunnelModeNameToValue = map[string]TunnelMode{
		"TunnelModeInactive": TunnelModeInactive,
		"TunnelModeActive":   TunnelModeActive,
	}

	_TunnelModeValueToName = map[TunnelMode]string{
		TunnelModeInactive: "TunnelModeInactive",
		TunnelModeActive:   "TunnelModeActive",
	}
)

func init() {
	var v TunnelMode
	if _, ok := interface{}(v).(fmt.Stringer); ok {
		_TunnelModeNameToValue = map[string]TunnelMode{
			interface{}(TunnelModeInactive).(fmt.Stringer).String(): TunnelModeInactive,
			interface{}(TunnelModeActive).(fmt.Stringer).String():   TunnelModeActive,
		}
	}
}

// MarshalJSON is generated so TunnelMode satisfies json.Marshaler.
func (r TunnelMode) MarshalJSON() ([]byte, error) {
	if s, ok := interface{}(r).(fmt.Stringer); ok {
		return json.Marshal(s.String())
	}
	s, ok := _TunnelModeValueToName[r]
	if !ok {
		return nil, fmt.Errorf("invalid TunnelMode: %d", r)
	}
	return json.Marshal(s)
}

// UnmarshalJSON is generated so TunnelMode satisfies json.Unmarshaler.
func (r *TunnelMode) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return fmt.Errorf("TunnelMode should be a string, got %s", data)
	}
	v, ok := _TunnelModeNameToValue[s]
	if !ok {
		return fmt.Errorf("invalid TunnelMode %q", s)
	}
	*r = v
	return nil
}