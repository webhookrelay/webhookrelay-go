package webhookrelay

import (
	"errors"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestStartOutboundImport(t *testing.T) {
	const secret = "whsec_MfKQ9r8GKYqrTwjUPD8ILPZIo2LaLaSw"
	api, requests := outboundServer(t, `{"id":"import-1","status":"pending","total":4,"processed":0,
		"event_types":{"total":1,"created":0,"unchanged":0},
		"consumers":{"total":1,"created":0,"unchanged":0},
		"endpoints":{"total":2,"created":0,"unchanged":0},
		"created_at":"2026-11-09T10:00:00Z","updated_at":"2026-11-09T10:00:00Z"}`)

	task, err := api.StartOutboundImport(&OutboundImportOptions{
		EventTypes: []OutboundImportEventType{{Name: "invoice.paid", Description: "Paid", Example: map[string]interface{}{"id": "inv_1"}}},
		Consumers:  []OutboundImportConsumer{{ID: "cust_42", Name: "Acme", Rate: 10}},
		Endpoints: []OutboundImportEndpoint{
			{Consumer: "cust_42", URL: "https://example.com/hook", EventTypes: []string{"*"}, Secret: secret, Paused: true, Headers: map[string]string{"X-A": "1"}},
			{
				Consumer:    "cust_42",
				Type:        OutboundEndpointTypeAWSSQS,
				Destination: &OutboundDestination{AWSSQS: &OutboundAWSSQS{QueueURL: "https://sqs.eu-west-1.amazonaws.com/1/orders"}},
				Credentials: &OutboundEndpointCredentials{AWS: &OutboundAWSCredentials{AccessKeyID: "AKIA", SecretAccessKey: "s3cr3t"}},
				EventTypes:  []string{"invoice.paid"},
			},
		},
	})
	if err != nil {
		t.Fatalf("StartOutboundImport: %v", err)
	}
	if task.ID != "import-1" || task.Status != OutboundImportStatusPending || task.Total != 4 || task.Endpoints.Total != 2 || task.Done() {
		t.Fatalf("unexpected task: %+v", task)
	}

	got := (*requests)[0]
	if got.Method != http.MethodPost || got.Path != "/outbound/imports" {
		t.Fatalf("unexpected request %s %s", got.Method, got.Path)
	}
	want := map[string]interface{}{
		"event_types": []interface{}{map[string]interface{}{"name": "invoice.paid", "description": "Paid", "example": map[string]interface{}{"id": "inv_1"}}},
		"consumers":   []interface{}{map[string]interface{}{"id": "cust_42", "name": "Acme", "rate": 10}},
		"endpoints": []interface{}{
			map[string]interface{}{"consumer": "cust_42", "url": "https://example.com/hook", "event_types": []interface{}{"*"}, "secret": secret, "paused": true, "headers": map[string]interface{}{"X-A": "1"}},
			map[string]interface{}{
				"consumer":    "cust_42",
				"type":        "aws_sqs",
				"destination": map[string]interface{}{"aws_sqs": map[string]interface{}{"queue_url": "https://sqs.eu-west-1.amazonaws.com/1/orders"}},
				"credentials": map[string]interface{}{"aws": map[string]interface{}{"access_key_id": "AKIA", "secret_access_key": "s3cr3t"}},
				"event_types": []interface{}{"invoice.paid"},
			},
		},
	}
	if !jsonEqual(got.Body, want) {
		t.Fatalf("unexpected body:\n got %v\nwant %v", got.Body, want)
	}
}

func TestGetOutboundImport(t *testing.T) {
	api, requests := outboundServer(t, `{"id":"import-1","status":"completed","total":3,"processed":3,
		"event_types":{"total":1,"created":1,"unchanged":0},
		"consumers":{"total":1,"created":0,"unchanged":1},
		"endpoints":{"total":1,"created":1,"unchanged":0},
		"created_at":"2026-11-09T10:00:00Z","updated_at":"2026-11-09T10:00:05Z","completed_at":"2026-11-09T10:00:05Z"}`)
	task, err := api.GetOutboundImport("import-1")
	if err != nil {
		t.Fatalf("GetOutboundImport: %v", err)
	}
	if got := (*requests)[0]; got.Method != http.MethodGet || got.Path != "/outbound/imports/import-1" {
		t.Fatalf("unexpected request %s %s", got.Method, got.Path)
	}
	if !task.Done() || task.Processed != 3 || task.Consumers != (OutboundImportCounts{Total: 1, Unchanged: 1}) ||
		task.EventTypes.Created != 1 || task.CompletedAt == nil || !task.CompletedAt.Equal(time.Date(2026, 11, 9, 10, 0, 5, 0, time.UTC)) {
		t.Fatalf("unexpected task: %+v", task)
	}
}

func TestGetOutboundImportFailedWithItemErrors(t *testing.T) {
	api, _ := outboundServer(t, `{"id":"import-1","status":"failed","error":"1 item refused",
		"errors":[{"kind":"endpoint","index":0,"key":"cust_42 https://internal.example","error":"host resolves to a private address"}]}`)
	task, err := api.GetOutboundImport("import-1")
	if err != nil {
		t.Fatalf("GetOutboundImport: %v", err)
	}
	want := []OutboundImportItemError{{Kind: OutboundImportItemEndpoint, Index: 0, Key: "cust_42 https://internal.example", Error: "host resolves to a private address"}}
	if !task.Done() || task.Error != "1 item refused" || !jsonEqual(task.Errors, want) {
		t.Fatalf("unexpected task: %+v", task)
	}
}

func TestStartOutboundImportItemErrors(t *testing.T) {
	api := errorServer(t, http.StatusBadRequest, `{"error":"4 import items are invalid","errors":[
		{"kind":"event_type","index":0,"key":"invoice.paid","error":"listed twice"},
		{"kind":"consumer","index":1,"key":"cust_7","error":"id is required"},
		{"kind":"function","index":0,"error":"a kind the API added later"},
		{"kind":"limit","index":-1,"key":"consumers","error":"too many items"}]}`, nil)
	_, err := api.StartOutboundImport(&OutboundImportOptions{})
	var apiErr *APIError
	if !errors.As(err, &apiErr) || !IsBadRequest(err) {
		t.Fatalf("expected a 400 APIError, got %v", err)
	}
	want := []OutboundImportItemError{
		{Kind: OutboundImportItemEventType, Index: 0, Key: "invoice.paid", Error: "listed twice"},
		{Kind: OutboundImportItemConsumer, Index: 1, Key: "cust_7", Error: "id is required"},
		{Kind: "function", Index: 0, Error: "a kind the API added later"},
		{Kind: OutboundImportItemLimit, Index: -1, Key: "consumers", Error: "too many items"},
	}
	if apiErr.Message != "4 import items are invalid" || !jsonEqual(apiErr.ImportErrors, want) {
		t.Fatalf("unexpected error: %+v", apiErr)
	}
}

func TestStartOutboundImportConflict(t *testing.T) {
	api := errorServer(t, http.StatusConflict, `{"error":"another import is running"}`, nil)
	_, err := api.StartOutboundImport(&OutboundImportOptions{})
	if !IsConflict(err) {
		t.Fatalf("expected a 409, got %v", err)
	}
}

// An "errors" field of another shape must neither hide the message nor pass
// as import items: strings fail to decode, objects decode with empty fields,
// and one such item refuses the whole list. An empty list is no list.
func TestAPIErrorIgnoresForeignErrorsShape(t *testing.T) {
	for _, errorsField := range []string{
		`["a","b"]`,
		`[{"field":"url","message":"required"}]`,
		`[{"kind":"endpoint","index":0,"error":"bad url"},{"code":"bad_gateway"}]`,
		`[{"kind":"limit","error":"too many endpoints"}]`, // no position
		`[{"kind":"endpoint","index":0,"error":""}]`,
		`[]`,
	} {
		api := errorServer(t, http.StatusBadRequest, `{"error":"bad","errors":`+errorsField+`}`, nil)
		_, err := api.StartOutboundImport(&OutboundImportOptions{})
		var apiErr *APIError
		if !errors.As(err, &apiErr) || apiErr.Message != "bad" || apiErr.ImportErrors != nil {
			t.Fatalf("errors %s: unexpected error %+v", errorsField, err)
		}
	}
}

// The wire values callers filter on are the server's.
func TestOutboundImportWireValues(t *testing.T) {
	got := []string{
		OutboundImportItemEventType, OutboundImportItemConsumer, OutboundImportItemEndpoint, OutboundImportItemLimit,
		OutboundImportStatusPending, OutboundImportStatusValidating, OutboundImportStatusRunning, OutboundImportStatusCompleted, OutboundImportStatusFailed,
		OutboundEndpointTypeWebhook, OutboundEndpointTypeAWSSQS, OutboundEndpointTypeAWSSNS, OutboundEndpointTypeAWSS3, OutboundEndpointTypeGCPPubSub, OutboundEndpointTypeGCPGCS,
	}
	want := []string{
		"event_type", "consumer", "endpoint", "limit",
		"pending", "validating", "running", "completed", "failed",
		"webhook", "aws_sqs", "aws_sns", "aws_s3", "gcp_pubsub", "gcp_gcs",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("wire values changed:\n got %q\nwant %q", got, want)
	}
}

// OutboundImportEndpoint repeats the OutboundEndpointOptions fields; a field
// added to one and not the other, or encoded differently, would silently
// change imports.
func TestOutboundImportEndpointCarriesEveryEndpointOption(t *testing.T) {
	// A cloud endpoint has no URL, so an import may omit it; creating a
	// webhook endpoint always sends one.
	differ := map[string]string{"url": "url,omitempty"}
	importFields := jsonFields(reflect.TypeOf(OutboundImportEndpoint{}))
	for name, option := range jsonFields(reflect.TypeOf(OutboundEndpointOptions{})) {
		got, ok := importFields[name]
		want := option.tag
		if tag, ok := differ[name]; ok {
			want = tag
		}
		if !ok || got.typ != option.typ || got.tag != want {
			t.Errorf("OutboundImportEndpoint field %q = %+v, want %s %q", name, got, option.typ, want)
		}
	}
}

type jsonField struct {
	typ reflect.Type
	tag string
}

// jsonFields maps a struct's JSON field names to their Go types and full
// tags, skipping fields that are not encoded.
func jsonFields(typ reflect.Type) map[string]jsonField {
	fields := map[string]jsonField{}
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		tag := field.Tag.Get("json")
		name, _, _ := strings.Cut(tag, ",")
		if name != "" && name != "-" {
			fields[name] = jsonField{typ: field.Type, tag: tag}
		}
	}
	return fields
}

func TestStartOutboundImportTooLarge(t *testing.T) {
	api := errorServer(t, http.StatusRequestEntityTooLarge, `{"error":"the import exceeds 10 MiB; split it into several imports"}`, nil)
	_, err := api.StartOutboundImport(nil)
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusRequestEntityTooLarge || apiErr.ImportErrors != nil {
		t.Fatalf("expected a 413 APIError without items, got %v", err)
	}
}

func TestOutboundImportDoneForEveryFinalStatus(t *testing.T) {
	for status, done := range map[string]bool{
		OutboundImportStatusPending: false, OutboundImportStatusValidating: false, OutboundImportStatusRunning: false,
		OutboundImportStatusCompleted: true, OutboundImportStatusFailed: true,
		"cancelled": true,  // a final status added after this release
		"":          false, // a task whose status could not be read
	} {
		if got := (&OutboundImportTask{Status: status}).Done(); got != done {
			t.Errorf("Done() for %q = %v, want %v", status, got, done)
		}
	}
}

func TestStartOutboundImportWithoutOptionsSendsAnEmptyImport(t *testing.T) {
	api, requests := outboundServer(t, `{"id":"import-1","status":"pending"}`)
	if _, err := api.StartOutboundImport(nil); err != nil {
		t.Fatalf("StartOutboundImport: %v", err)
	}
	if got := (*requests)[0].Body; !jsonEqual(got, map[string]interface{}{}) {
		t.Fatalf("expected an empty import object, got %#v", got)
	}
}

func TestGetOutboundImportEscapesTheID(t *testing.T) {
	api, requests := outboundServer(t, `{"id":"import/1","status":"running"}`)
	if _, err := api.GetOutboundImport("import/1"); err != nil {
		t.Fatalf("GetOutboundImport: %v", err)
	}
	if got := (*requests)[0].Path; got != "/outbound/imports/import%2F1" {
		t.Fatalf("path = %s", got)
	}
}
