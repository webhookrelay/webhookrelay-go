package webhookrelay

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"

	"github.com/pkg/errors"

	reactor_v1 "github.com/webhookrelay/webhookrelay-go/api/reactor/v1"
)

// Function is an alias to reactor_v1 pkg
type Function = reactor_v1.Function

// ExecuteResponse is an alias to reactor v1 pkg
type ExecuteResponse = reactor_v1.ExecuteResponse

// FunctionRequest used for creating/updating functions
type FunctionRequest struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Payload string `json:"payload"`
	Driver  string `json:"driver"`
}

// CreateFunctionRequest is used when creating a new function
type CreateFunctionRequest struct {
	Name    string
	Driver  string
	Payload io.Reader
}

// UpdateFunctionRequest is used when updating an existing function
type UpdateFunctionRequest struct {
	ID      string
	Name    string
	Driver  string
	Payload io.Reader
}

// InvokeFunctionRequest is a function invoke payload
type InvokeFunctionRequest struct {
	Headers     map[string][]string `json:"headers"`
	RawQuery    string              `json:"raw_query"`
	RequestBody string              `json:"request_body"`
	Method      string              `json:"method"`
}

// InvokeOpts used to invoke functions, carries function ID
// and payload
type InvokeOpts struct {
	ID                    string
	InvokeFunctionRequest InvokeFunctionRequest
}

// FunctionListOptions is used to list functions
type FunctionListOptions struct{}

// ListFunctions lists functions for an account
func (api *API) ListFunctions(options *FunctionListOptions) ([]*Function, error) {
	resp, err := api.makeRequest(http.MethodGet, "/functions", nil)
	if err != nil {
		return nil, errors.Wrap(err, errMakeRequestError)
	}

	var functions []*reactor_v1.Function
	err = json.Unmarshal(resp, &functions)
	if err != nil {
		return nil, errors.Wrap(err, errUnmarshalError)
	}

	return functions, nil
}

// InvokeFunction invokes function and gets a response
func (api *API) InvokeFunction(options *InvokeOpts) (*ExecuteResponse, error) {

	resp, err := api.makeRequest("POST", "/functions/"+options.ID+"/invoke", options.InvokeFunctionRequest)
	if err != nil {
		return nil, err
	}

	var f ExecuteResponse
	if err := json.Unmarshal(resp, &f); err != nil {
		return nil, err
	}
	return &f, nil
}

// GetFunction - get function by ref
func (api *API) GetFunction(ref string) (*Function, error) {

	ref, err := api.ensureFunctionID(ref)
	if err != nil {
		return nil, err
	}

	resp, err := api.makeRequest("GET", "/functions/"+ref, nil)
	if err != nil {
		return nil, err
	}

	var function Function
	if err := json.Unmarshal(resp, &function); err != nil {
		return nil, err
	}
	return &function, nil
}

// CreateFunction - create new function
func (api *API) CreateFunction(opts *CreateFunctionRequest) (*Function, error) {

	functionBody, err := ioutil.ReadAll(opts.Payload)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read function body")
	}

	createOpts := &FunctionRequest{
		Name:    opts.Name,
		Driver:  opts.Driver,
		Payload: base64.StdEncoding.EncodeToString(functionBody),
	}
	// TODO: consider splitting function uploading and creation into separate reqs
	resp, err := api.makeRequest("POST", "/functions", createOpts)
	if err != nil {
		return nil, err
	}

	var f reactor_v1.Function
	if err := json.Unmarshal(resp, &f); err != nil {
		return nil, err
	}
	return &f, nil
}

// UpdateFunction - update function
func (api *API) UpdateFunction(options *UpdateFunctionRequest) (*Function, error) {

	if options.ID != "" {
		// ok
	} else if options.Name != "" {
		fID, err := api.ensureFunctionID(options.ID)
		if err != nil {
			return nil, err
		}
		options.ID = fID
	} else {
		return nil, fmt.Errorf("either name or ID has to be set")
	}

	functionBody, err := ioutil.ReadAll(options.Payload)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read function body")
	}

	updateOpts := &FunctionRequest{
		Name:    options.Name,
		Driver:  options.Driver,
		Payload: base64.StdEncoding.EncodeToString(functionBody),
	}

	resp, err := api.makeRequest("PUT", "/functions/"+options.ID, updateOpts)
	if err != nil {
		return nil, err
	}
	var function reactor_v1.Function
	if err := json.Unmarshal(resp, &function); err != nil {
		return nil, err
	}
	return &function, nil
}

// FunctionDeleteOptions is used in function delete request
type FunctionDeleteOptions struct {
	ID string `json:"id"`
}

// DeleteFunction - delete function
func (api *API) DeleteFunction(options *FunctionDeleteOptions) error {
	if options.ID == "" {
		return fmt.Errorf("ID must be supplied")
	}

	id, err := api.ensureFunctionID(options.ID)
	if err != nil {
		return err
	}
	options.ID = id

	_, err = api.makeRequest("DELETE", "/functions/"+options.ID, nil)
	if err != nil {
		return err
	}

	return nil
}

// ensureFunctionID - takes name/id and always returns ID (when it not fails)
func (api *API) ensureFunctionID(ref string) (string, error) {
	if !IsUUID(ref) {
		id, err := api.functionIDFromRef(ref)
		if err != nil {
			return "", err
		}
		return id, nil
	}
	return ref, nil
}

func (api *API) functionIDFromRef(ref string) (id string, err error) {
	functions, err := api.ListFunctions(&FunctionListOptions{})
	if err != nil {
		return
	}
	for _, f := range functions {
		if f.Id == ref || f.Name == ref {
			return f.Id, nil
		}
	}
	return "", fmt.Errorf("no such function '%s'", ref)
}
