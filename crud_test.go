package webhookrelay

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestDeleteBucketForceQueryParam verifies DeleteBucket forwards the Force flag
// as a "force" query parameter (the API returns 412 without it when the bucket
// still has inputs/outputs).
func TestDeleteBucketForceQueryParam(t *testing.T) {
	const bucketID = "11111111-1111-1111-1111-111111111111"
	var gotMethod, gotPath, gotQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath, gotQuery = r.Method, r.URL.Path, r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"` + bucketID + `","status":"deleted"}`))
	}))
	defer server.Close()

	client, err := New("k", "s", WithAPIEndpointURL(server.URL))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// Force set -> force=true query present.
	if err := client.DeleteBucket(&BucketDeleteOptions{Ref: bucketID, Force: true}); err != nil {
		t.Fatalf("DeleteBucket(force): %v", err)
	}
	if gotMethod != http.MethodDelete || gotPath != "/buckets/"+bucketID {
		t.Fatalf("unexpected request: %s %s", gotMethod, gotPath)
	}
	if gotQuery != "force=true" {
		t.Fatalf("expected query %q, got %q", "force=true", gotQuery)
	}

	// Force unset -> no query string.
	gotQuery = "sentinel"
	if err := client.DeleteBucket(&BucketDeleteOptions{Ref: bucketID}); err != nil {
		t.Fatalf("DeleteBucket: %v", err)
	}
	if gotQuery != "" {
		t.Fatalf("expected no query without force, got %q", gotQuery)
	}
}

// TestListFunctionConfigurationVariablesParsesArray verifies the config list
// endpoint response (a bare JSON array of variables) is parsed correctly.
func TestListFunctionConfigurationVariablesParsesArray(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/functions/fn-1/config" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"key":"A","value":"1","created_at":"2023-11-14T22:13:20Z"},{"key":"B","value":"2"}]`))
	}))
	defer server.Close()

	client, err := New("k", "s", WithAPIEndpointURL(server.URL))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	vars, err := client.ListFunctionConfigurationVariables(&FunctionConfigurationVariablesListOptions{ID: "fn-1"})
	if err != nil {
		t.Fatalf("ListFunctionConfigurationVariables: %v", err)
	}
	if len(vars) != 2 || vars[0].Key != "A" || vars[1].Key != "B" {
		t.Fatalf("unexpected variables: %+v", vars)
	}
	if vars[0].Value != "1" || vars[1].Value != "2" {
		t.Fatalf("values not parsed: %+v", vars)
	}
}
