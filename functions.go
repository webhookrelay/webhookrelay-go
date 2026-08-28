package webhookrelay

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/pkg/errors"
)

// Function represents a reactor function (transformation) that can be attached
// to bucket inputs/outputs to modify webhooks in flight.
type Function struct {
	ID        string `json:"id"`
	AccountID string `json:"account_id"`
	Created   int64  `json:"created"`
	Updated   int64  `json:"updated"`
	// Driver specifies which runtime executes the function, e.g. "js" or "lua".
	Driver string `json:"driver"`
	// Payload is the function source/executable. When creating or updating a
	// function via the SDK it is base64 encoded before being sent.
	Payload     string `json:"payload"`
	Name        string `json:"name"`
	PayloadSize int64  `json:"payload_size"`
	// Compression type (if any) for the payload, for example gzip or zlib.
	Compression string            `json:"compression"`
	Metadata    map[string]string `json:"metadata"`
}

// ExecuteResponse is returned when a function is invoked. It describes how the
// function modified the request/response and whether forwarding should stop.
type ExecuteResponse struct {
	RequestID        string    `json:"request_id"`
	RequestModified  bool      `json:"request_modified"`
	ResponseModified bool      `json:"response_modified"`
	Request          *Request  `json:"request"`
	Response         *Response `json:"response"`
	FunctionID       string    `json:"function_id"`
	// Error is the driver error (set when the function failed to execute).
	Error          string `json:"error"`
	StopForwarding bool   `json:"stop_forwarding"`
}

// Request is the (possibly modified) HTTP request as seen by a function.
type Request struct {
	Body             []byte            `json:"body"`
	BodyModified     bool              `json:"body_modified"`
	Header           map[string]Header `json:"header"`
	HeaderModified   bool              `json:"header_modified"`
	Path             string            `json:"path"`
	PathModified     bool              `json:"path_modified"`
	RawQuery         string            `json:"raw_query"`
	RawQueryModified bool              `json:"raw_query_modified"`
	Method           string            `json:"method"`
	MethodModified   bool              `json:"method_modified"`
	Metadata         map[string]string `json:"metadata"`
	BodyPath         string            `json:"body_path"`
	ModifiedBodyPath string            `json:"modified_body_path"`
}

// Response is an optional response a function can set to return a dynamic reply.
type Response struct {
	Status   int32             `json:"status"`
	Body     []byte            `json:"body"`
	Header   map[string]Header `json:"header"`
	BodyPath string            `json:"body_path"`
}

// Header holds the values for a single HTTP header key.
type Header struct {
	Values []string `json:"values"`
}

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

	var functions []*Function
	err = json.Unmarshal(resp, &functions)
	if err != nil {
		return nil, errors.Wrap(err, errUnmarshalError)
	}

	return functions, nil
}

// InvokeFunction invokes function and gets a response
func (api *API) InvokeFunction(options *InvokeOpts) (*ExecuteResponse, error) {

	// Resolve a name to an ID like GetFunction/UpdateFunction/DeleteFunction
	// do; before this, invoking by name 404ed while every sibling accepted it.
	id, err := api.ensureFunctionID(options.ID)
	if err != nil {
		return nil, err
	}

	resp, err := api.makeRequest("POST", "/functions/"+id+"/invoke", options.InvokeFunctionRequest)
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

	functionBody, err := io.ReadAll(opts.Payload)
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

	var f Function
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

	functionBody, err := io.ReadAll(options.Payload)
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
	var function Function
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
		if f.ID == ref || f.Name == ref {
			return f.ID, nil
		}
	}
	return "", fmt.Errorf("no such function '%s'", ref)
}
