//go:generate jsonenums -type=AuthType
package client

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
