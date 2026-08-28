package webhookrelay

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestInvokeFunctionResolvesNamesLikeItsSiblings: GetFunction, UpdateFunction
// and DeleteFunction all accept a name or an ID; InvokeFunction used to put
// the ref straight into the URL, so invoking by name 404ed. Callers testing a
// function they just created by name is the common case.
func TestInvokeFunctionResolvesNamesLikeItsSiblings(t *testing.T) {
	const fnID = "5cd3ca6c-46a8-4831-a4cc-e29793b0c821"

	var invokedPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/functions":
			json.NewEncoder(w).Encode([]*Function{{ID: fnID, Name: "greet", Driver: "js"}})
		case r.Method == http.MethodPost:
			invokedPath = r.URL.Path
			json.NewEncoder(w).Encode(ExecuteResponse{FunctionID: fnID})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	client, err := NewWithAPIKey("sk-test", WithAPIEndpointURL(server.URL))
	if err != nil {
		t.Fatal(err)
	}

	// By name: must resolve to the ID before hitting the invoke endpoint.
	if _, err := client.InvokeFunction(&InvokeOpts{
		ID:                    "greet",
		InvokeFunctionRequest: InvokeFunctionRequest{RequestBody: "{}", Method: "POST"},
	}); err != nil {
		t.Fatalf("invoking by name: %v", err)
	}
	if invokedPath != "/functions/"+fnID+"/invoke" {
		t.Fatalf("name was not resolved to an ID: %s", invokedPath)
	}

	// By ID: no resolution round trip changes the path.
	invokedPath = ""
	if _, err := client.InvokeFunction(&InvokeOpts{
		ID:                    fnID,
		InvokeFunctionRequest: InvokeFunctionRequest{RequestBody: "{}", Method: "POST"},
	}); err != nil {
		t.Fatalf("invoking by id: %v", err)
	}
	if invokedPath != "/functions/"+fnID+"/invoke" {
		t.Fatalf("id path wrong: %s", invokedPath)
	}

	// An unknown name is a clear error, not a 404 on a name-shaped URL.
	if _, err := client.InvokeFunction(&InvokeOpts{ID: "no-such-function"}); err == nil {
		t.Fatal("an unknown name did not error")
	}
}
