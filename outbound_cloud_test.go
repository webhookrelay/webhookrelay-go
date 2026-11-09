package webhookrelay

import (
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"testing"
)

var (
	testAWSCredentials = &OutboundEndpointCredentials{AWS: &OutboundAWSCredentials{AccessKeyID: "AKIAEXAMPLE", SecretAccessKey: "aws-secret"}}
	testGCPCredentials = &OutboundEndpointCredentials{GCP: &OutboundGCPCredentials{ServiceAccountKey: `{"type":"service_account","private_key":"gcp-secret"}`}}
)

// Each cloud type sends its type, its one destination block and its
// credentials, and no URL or headers.
func TestCreateOutboundCloudEndpoint(t *testing.T) {
	for _, tc := range []struct {
		typ         string
		destination *OutboundDestination
		credentials *OutboundEndpointCredentials
		wantBlock   string
	}{
		{OutboundEndpointTypeAWSSQS, &OutboundDestination{AWSSQS: &OutboundAWSSQS{QueueURL: "https://sqs.eu-west-1.amazonaws.com/123456789012/orders"}}, testAWSCredentials,
			`"aws_sqs":{"queue_url":"https://sqs.eu-west-1.amazonaws.com/123456789012/orders"}`},
		{OutboundEndpointTypeAWSSNS, &OutboundDestination{AWSSNS: &OutboundAWSSNS{TopicARN: "arn:aws:sns:eu-west-1:123456789012:orders", Region: "eu-west-1"}}, testAWSCredentials,
			`"aws_sns":{"topic_arn":"arn:aws:sns:eu-west-1:123456789012:orders","region":"eu-west-1"}`},
		{OutboundEndpointTypeAWSS3, &OutboundDestination{AWSS3: &OutboundAWSS3{Bucket: "orders", Region: "auto", Prefix: "in", EndpointURL: "https://acct.r2.cloudflarestorage.com"}}, testAWSCredentials,
			`"aws_s3":{"bucket":"orders","region":"auto","prefix":"in","endpoint_url":"https://acct.r2.cloudflarestorage.com"}`},
		{OutboundEndpointTypeGCPPubSub, &OutboundDestination{GCPPubSub: &OutboundGCPPubSub{Topic: "projects/p/topics/orders"}}, testGCPCredentials,
			`"gcp_pubsub":{"topic":"projects/p/topics/orders"}`},
		{OutboundEndpointTypeGCPGCS, &OutboundDestination{GCPGCS: &OutboundGCPGCS{Bucket: "orders", Prefix: "in"}}, testGCPCredentials,
			`"gcp_gcs":{"bucket":"orders","prefix":"in"}`},
	} {
		t.Run(tc.typ, func(t *testing.T) {
			api, requests := outboundServer(t, `{"id":"ep-1","consumer":"cust_42","type":"`+tc.typ+`","url":"derived","destination":{`+tc.wantBlock+`},"credentials_identity":"identity","secret":"whsec_abc"}`)
			endpoint, err := api.CreateOutboundEndpoint(&OutboundEndpointOptions{
				Consumer:    "cust_42",
				Type:        tc.typ,
				Destination: tc.destination,
				Credentials: tc.credentials,
				EventTypes:  []string{"*"},
			})
			if err != nil {
				t.Fatalf("CreateOutboundEndpoint: %v", err)
			}
			req := (*requests)[0]
			if req.Method != http.MethodPost || req.Path != "/outbound/consumers/cust_42/endpoints" {
				t.Fatalf("unexpected request %s %s", req.Method, req.Path)
			}
			var want map[string]interface{}
			credentials, _ := json.Marshal(tc.credentials)
			wantJSON := `{"type":"` + tc.typ + `","url":"","destination":{` + tc.wantBlock + `},"credentials":` + string(credentials) + `,"event_types":["*"]}`
			if err := json.Unmarshal([]byte(wantJSON), &want); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(req.Body, want) {
				t.Fatalf("body = %v\nwant   %v", req.Body, want)
			}
			if endpoint.Type != tc.typ || !reflect.DeepEqual(endpoint.Destination, tc.destination) || endpoint.CredentialsIdentity != "identity" {
				t.Fatalf("unexpected endpoint %+v", endpoint)
			}
		})
	}
}

// An update without credentials keeps the stored ones: nothing is sent.
func TestUpdateOutboundCloudEndpointKeepsCredentials(t *testing.T) {
	api, requests := outboundServer(t, `{"id":"ep-1","type":"aws_sqs"}`)
	_, err := api.UpdateOutboundEndpoint("ep-1", &OutboundEndpointOptions{
		Type:        OutboundEndpointTypeAWSSQS,
		Destination: &OutboundDestination{AWSSQS: &OutboundAWSSQS{QueueURL: "https://sqs.eu-west-1.amazonaws.com/123456789012/orders"}},
		EventTypes:  []string{"*"},
	})
	if err != nil {
		t.Fatalf("UpdateOutboundEndpoint: %v", err)
	}
	req := (*requests)[0]
	if req.Method != http.MethodPut || req.Path != "/outbound/endpoints/ep-1" {
		t.Fatalf("unexpected request %s %s", req.Method, req.Path)
	}
	if _, sent := req.Body["credentials"]; sent {
		t.Fatalf("an update without credentials must not send them: %v", req.Body)
	}
}

// Credentials are write-only: no type an API response decodes into can hold
// them, so a server bug or a proxy could not surface them through the SDK.
func TestCredentialsAreNeverReadBack(t *testing.T) {
	for _, typ := range []reflect.Type{
		reflect.TypeOf(OutboundEndpoint{}), reflect.TypeOf(OutboundDestination{}),
		reflect.TypeOf(OutboundEndpointStats{}), reflect.TypeOf(OutboundImportTask{}),
	} {
		assertNoCredentials(t, typ, typ.Name())
	}

	var endpoint OutboundEndpoint
	body := `{"id":"ep-1","type":"aws_sqs","credentials":{"aws":{"access_key_id":"AKIA","secret_access_key":"aws-secret"}},"credentials_identity":"AKIA"}`
	if err := json.Unmarshal([]byte(body), &endpoint); err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(endpoint)
	if strings.Contains(string(encoded), "aws-secret") {
		t.Fatalf("credentials survived a read: %s", encoded)
	}
	if endpoint.CredentialsIdentity != "AKIA" {
		t.Fatalf("credentials_identity = %q", endpoint.CredentialsIdentity)
	}
}

func assertNoCredentials(t *testing.T, typ reflect.Type, path string) {
	t.Helper()
	for typ.Kind() == reflect.Pointer || typ.Kind() == reflect.Slice || typ.Kind() == reflect.Map {
		typ = typ.Elem()
	}
	if typ.Kind() != reflect.Struct || typ.PkgPath() != reflect.TypeOf(API{}).PkgPath() {
		return
	}
	if typ == reflect.TypeOf(OutboundEndpointCredentials{}) {
		t.Errorf("%s can hold credentials", path)
		return
	}
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		assertNoCredentials(t, field.Type, path+"."+field.Name)
	}
}
