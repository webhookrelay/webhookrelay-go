package webhookrelay

import (
	"context"
	"encoding/json"
)

// VersionInfo describes version and runtime info.
type VersionInfo struct {
	Name          string `json:"name"`
	BuildDate     string `json:"buildDate"`
	Revision      string `json:"revision"`
	Version       string `json:"version"`
	APIVersion    string `json:"apiVersion"`
	GoVersion     string `json:"goVersion"`
	OS            string `json:"os"`
	Arch          string `json:"arch"`
	KernelVersion string `json:"kernelVersion"`
	Experimental  bool   `json:"experimental"`
}

// ServerVersion is an alias for VersionInfo for backward compatibility.
type ServerVersion = VersionInfo

// GetVersion returns the server's version and runtime info without requiring a context.
func (api *API) GetVersion() (*ServerVersion, error) {
	return api.ServerVersion(context.TODO())
}

// ServerVersion returns the server's version and runtime info.
func (api *API) ServerVersion(ctx context.Context) (*VersionInfo, error) {

	resp, err := api.makeRequest("GET", "/version", nil)
	if err != nil {
		return nil, err
	}

	var version VersionInfo
	if err := json.Unmarshal(resp, &version); err != nil {
		return nil, err
	}
	return &version, nil
}
