package webhookrelay

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/pkg/errors"
	reactor_v1 "github.com/webhookrelay/webhookrelay-go/api/reactor/v1"
)

// ListConfigResponse defines function config
type ListConfigResponse = reactor_v1.ListConfigResponse

// Variable is function configuration variable
type Variable = reactor_v1.Variable

// SetFunctionConfigRequest sets/updates function configuration
type SetFunctionConfigRequest struct {
	ID    string `json:"-"` // function ID
	Key   string `json:"key"`
	Value string `json:"value"`
}

// FunctionConfigurationVariablesListOptions is used to list function config variables
type FunctionConfigurationVariablesListOptions struct {
	ID string
}

// ListFunctionConfigurationVariables lists function configuration variables
func (api *API) ListFunctionConfigurationVariables(options *FunctionConfigurationVariablesListOptions) ([]*Variable, error) {
	resp, err := api.makeRequest(http.MethodGet, "/functions/"+options.ID+"/config", nil)
	if err != nil {
		return nil, errors.Wrap(err, errMakeRequestError)
	}

	var result *ListConfigResponse
	err = json.Unmarshal(resp, &result)
	if err != nil {
		return nil, errors.Wrap(err, errUnmarshalError)
	}

	return result.Variables, nil
}

// SetFunctionConfigurationVariable allows users to set config variables for a function. Function can then use special methods
// to retrieve those variables during runtime.
func (api *API) SetFunctionConfigurationVariable(options *SetFunctionConfigRequest) (*Variable, error) {

	resp, err := api.makeRequest("PUT", "/functions/"+options.ID+"/config", options)
	if err != nil {
		return nil, err
	}

	var result Variable
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// FunctionConfigurationVariableDeleteOptions is used in function configuration variable delete request
type FunctionConfigurationVariableDeleteOptions struct {
	ID  string
	Key string
}

// DeleteFunctionConfigurationVariable - delete function configuration variable
func (api *API) DeleteFunctionConfigurationVariable(options *FunctionConfigurationVariableDeleteOptions) error {
	if options.ID == "" {
		return fmt.Errorf("ID must be supplied")
	}

	if options.Key == "" {
		return fmt.Errorf("Key must be supplied")
	}

	id, err := api.ensureFunctionID(options.ID)
	if err != nil {
		return err
	}
	options.ID = id

	path := url.PathEscape("/functions/" + options.ID + "/config/" + options.Key)

	_, err = api.makeRequest("DELETE", path, nil)
	if err != nil {
		return err
	}

	return nil
}
