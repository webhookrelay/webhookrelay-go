package webhookrelay

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// integrationClient returns a live API client for integration tests, skipping
// the test when no credentials are configured. Set RELAY_API_KEY (a single API
// key, sent as a Bearer token) or the RELAY_KEY + RELAY_SECRET pair.
func integrationClient(t *testing.T) *API {
	t.Helper()
	client, err := getIntegrationTestClient()
	if err != nil {
		t.Skipf("skipping live API integration test: %v", err)
	}
	return client
}

// testNamePrefix is prepended to every top-level resource (buckets, functions)
// created by these live tests so they are easy to spot and never mix with
// resources created by the JavaScript SDK's tests.
const testNamePrefix = "test-sdk-go-"

// uniqueName builds a collision-resistant, prefixed resource name for a test run
// so parallel/re-run invocations do not clash on the live API and the resources
// are clearly identifiable as this SDK's.
func uniqueName(kind string) string {
	return fmt.Sprintf("%s%s-%d", testNamePrefix, kind, time.Now().UnixNano())
}

// TestIntegrationBucketLifecycle exercises the full resource pipeline against a
// live API in one flow: bucket -> input -> output -> function + configuration
// variables -> email service-connection input (email inbox). Every resource is
// cleaned up on completion; the bucket is deleted with Force so nothing leaks
// even if a sub-step fails.
func TestIntegrationBucketLifecycle(t *testing.T) {
	client := integrationClient(t)

	// --- Bucket ---
	bucketName := uniqueName("bucket")
	bucket, err := client.CreateBucket(&BucketCreateOptions{
		Name:        bucketName,
		Description: "SDK integration test bucket",
	})
	if err != nil {
		t.Fatalf("CreateBucket: %v", err)
	}
	t.Cleanup(func() {
		if err := client.DeleteBucket(&BucketDeleteOptions{Ref: bucket.ID, Force: true}); err != nil {
			t.Errorf("cleanup DeleteBucket(%s): %v", bucket.ID, err)
		}
	})
	if bucket.ID == "" || bucket.Name != bucketName {
		t.Fatalf("unexpected bucket: %+v", bucket)
	}

	// Round-trip via GetBucket (also exercises name/ID resolution).
	got, err := client.GetBucket(bucket.ID)
	if err != nil {
		t.Fatalf("GetBucket: %v", err)
	}
	if got.ID != bucket.ID {
		t.Fatalf("GetBucket returned %q, want %q", got.ID, bucket.ID)
	}

	// --- Input ---
	t.Run("input", func(t *testing.T) {
		input, err := client.CreateInput(&Input{
			BucketID:    bucket.ID,
			Name:        "primary",
			Description: "primary input",
			StatusCode:  200,
			Body:        "ok",
		})
		if err != nil {
			t.Fatalf("CreateInput: %v", err)
		}
		if input.ID == "" {
			t.Fatalf("input has no ID: %+v", input)
		}

		input.Description = "updated input"
		updated, err := client.UpdateInput(input)
		if err != nil {
			t.Fatalf("UpdateInput: %v", err)
		}
		if updated.Description != "updated input" {
			t.Errorf("input description not updated: %q", updated.Description)
		}

		inputs, err := client.ListInputs(&InputListOptions{Bucket: bucket.ID})
		if err != nil {
			t.Fatalf("ListInputs: %v", err)
		}
		if !containsInput(inputs, input.ID) {
			t.Errorf("created input %s not found in list", input.ID)
		}

		if err := client.DeleteInput(&InputDeleteOptions{Bucket: bucket.ID, Input: input.ID}); err != nil {
			t.Errorf("DeleteInput: %v", err)
		}
	})

	// --- Output ---
	t.Run("output", func(t *testing.T) {
		output, err := client.CreateOutput(&Output{
			BucketID:    bucket.ID,
			Name:        "primary-out",
			Destination: "https://example.com/hook",
			Retries:     -1,
		})
		if err != nil {
			t.Fatalf("CreateOutput: %v", err)
		}
		if output.ID == "" {
			t.Fatalf("output has no ID: %+v", output)
		}

		output.Description = "updated output"
		updated, err := client.UpdateOutput(output)
		if err != nil {
			t.Fatalf("UpdateOutput: %v", err)
		}
		if updated.Description != "updated output" {
			t.Errorf("output description not updated: %q", updated.Description)
		}

		outputs, err := client.ListOutputs(&OutputListOptions{Bucket: bucket.ID})
		if err != nil {
			t.Fatalf("ListOutputs: %v", err)
		}
		if !containsOutput(outputs, output.ID) {
			t.Errorf("created output %s not found in list", output.ID)
		}

		if err := client.DeleteOutput(&OutputDeleteOptions{Bucket: bucket.ID, Output: output.ID}); err != nil {
			t.Errorf("DeleteOutput: %v", err)
		}
	})

	// --- Function + configuration variables ---
	t.Run("function_config", func(t *testing.T) {
		const src = "function transform(r) { return r; }"
		fn, err := client.CreateFunction(&CreateFunctionRequest{
			Name:    uniqueName("fn"),
			Driver:  "js",
			Payload: strings.NewReader(src),
		})
		if err != nil {
			t.Fatalf("CreateFunction: %v", err)
		}
		t.Cleanup(func() {
			if err := client.DeleteFunction(&FunctionDeleteOptions{ID: fn.ID}); err != nil {
				t.Errorf("cleanup DeleteFunction(%s): %v", fn.ID, err)
			}
		})

		// The payload must round-trip as raw source, not base64.
		fetched, err := client.GetFunction(fn.ID)
		if err != nil {
			t.Fatalf("GetFunction: %v", err)
		}
		if !strings.Contains(fetched.Payload, "function transform") {
			t.Errorf("function payload is not raw source: %q", fetched.Payload)
		}

		if _, err := client.SetFunctionConfigurationVariable(&SetFunctionConfigRequest{
			ID: fn.ID, Key: "API_TOKEN", Value: "s3cr3t",
		}); err != nil {
			t.Fatalf("SetFunctionConfigurationVariable: %v", err)
		}

		vars, err := client.ListFunctionConfigurationVariables(&FunctionConfigurationVariablesListOptions{ID: fn.ID})
		if err != nil {
			t.Fatalf("ListFunctionConfigurationVariables: %v", err)
		}
		if !containsVariableKey(vars, "API_TOKEN") {
			t.Errorf("config variable API_TOKEN not found: %+v", vars)
		}

		if err := client.DeleteFunctionConfigurationVariable(&FunctionConfigurationVariableDeleteOptions{
			ID: fn.ID, Key: "API_TOKEN",
		}); err != nil {
			t.Errorf("DeleteFunctionConfigurationVariable: %v", err)
		}
	})

	// --- Email inbox (service-connection input of type email) ---
	t.Run("email_input", func(t *testing.T) {
		emailInput, err := client.CreateServiceConnectionInput(&ServiceConnectionInput{
			BucketID:                   bucket.ID,
			Name:                       "email-inbox",
			ServiceConnectionInputType: ServiceConnectionInputTypeEmail,
			EmailInput: EmailInput{
				Enabled:        true,
				AllowedSenders: []string{"alerts@example.com"},
			},
		})
		if err != nil {
			if isFeatureUnavailable(err) {
				t.Skipf("email-to-webhook not enabled for this account: %v", err)
			}
			t.Fatalf("CreateServiceConnectionInput(email): %v", err)
		}
		t.Cleanup(func() {
			if err := client.DeleteServiceConnectionInput(bucket.ID, emailInput.ID); err != nil {
				t.Errorf("cleanup DeleteServiceConnectionInput(%s): %v", emailInput.ID, err)
			}
		})

		if emailInput.ServiceConnectionInputType != ServiceConnectionInputTypeEmail {
			t.Errorf("unexpected input type: %q", emailInput.ServiceConnectionInputType)
		}
		// EmailAddress is populated only when the environment has an inbound
		// domain configured, so treat it as informational rather than required.
		if emailInput.EmailAddress == "" {
			t.Logf("email input created without an inbound address (inbound domain not configured for this environment)")
		} else if !strings.HasPrefix(emailInput.EmailAddress, emailInput.ID+"@") {
			t.Errorf("email_address %q does not start with input ID %q", emailInput.EmailAddress, emailInput.ID)
		}

		inputs, err := client.ListServiceConnectionInputs(bucket.ID)
		if err != nil {
			t.Fatalf("ListServiceConnectionInputs: %v", err)
		}
		found := false
		for _, in := range inputs {
			if in.ID == emailInput.ID {
				found = true
			}
		}
		if !found {
			t.Errorf("created email input %s not found in list", emailInput.ID)
		}

		// GetBucket must still decode once the bucket carries nested
		// service-connection resources (these use RFC3339 timestamps, while the
		// bucket's own timestamps are unix seconds — both must parse).
		withNested, err := client.GetBucket(bucket.ID)
		if err != nil {
			t.Fatalf("GetBucket with nested service-connection inputs: %v", err)
		}
		nestedFound := false
		for _, in := range withNested.ServiceConnectionInputs {
			if in.ID == emailInput.ID {
				nestedFound = true
			}
		}
		if !nestedFound {
			t.Errorf("email input %s not present in bucket.ServiceConnectionInputs", emailInput.ID)
		}
	})
}

// TestIntegrationCronLifecycle exercises cron CRUD against a live API. It
// verifies the Cron marshaler produces a payload the server accepts (no empty
// bucket object, no year-1 time bounds) and that responses decode.
func TestIntegrationCronLifecycle(t *testing.T) {
	client := integrationClient(t)

	cronName := uniqueName("cron")
	cron, err := client.CreateCron(&Cron{
		Name:        cronName,
		Schedule:    "0 0 * * *",
		Timezone:    "UTC",
		Method:      "POST",
		Destination: "https://example.com/hook",
		Payload:     `{"ping":true}`,
		// Leave disabled so it never actually fires during the test run.
		Enabled: false,
	})
	if err != nil {
		if isFeatureUnavailable(err) {
			t.Skipf("crons not enabled for this account: %v", err)
		}
		t.Fatalf("CreateCron: %v", err)
	}
	t.Cleanup(func() {
		if err := client.DeleteCron(cron.ID); err != nil {
			t.Errorf("cleanup DeleteCron(%s): %v", cron.ID, err)
		}
		// A cron provisions a bucket named after itself; remove it in case the
		// cron delete did not cascade (ignore "not found").
		_ = client.DeleteBucket(&BucketDeleteOptions{Ref: cronName, Force: true})
	})

	if cron.ID == "" || cron.Name != cronName {
		t.Fatalf("unexpected cron: %+v", cron)
	}

	got, err := client.GetCron(cron.ID)
	if err != nil {
		t.Fatalf("GetCron: %v", err)
	}
	if got.ID != cron.ID || got.Schedule != "0 0 * * *" || got.Destination != "https://example.com/hook" {
		t.Errorf("GetCron mismatch: %+v", got)
	}

	crons, err := client.ListCrons(&CronListOptions{})
	if err != nil {
		t.Fatalf("ListCrons: %v", err)
	}
	found := false
	for _, c := range crons {
		if c.ID == cron.ID {
			found = true
		}
	}
	if !found {
		t.Errorf("created cron %s not found in list", cron.ID)
	}
}

func containsInput(inputs []*Input, id string) bool {
	for _, in := range inputs {
		if in.ID == id {
			return true
		}
	}
	return false
}

func containsOutput(outputs []*Output, id string) bool {
	for _, o := range outputs {
		if o.ID == id {
			return true
		}
	}
	return false
}

func containsVariableKey(vars []*Variable, key string) bool {
	for _, v := range vars {
		if v.Key == key {
			return true
		}
	}
	return false
}

// isFeatureUnavailable reports whether err looks like a subscription/permission
// gate rather than a genuine failure, so optional feature tests can skip instead
// of failing on accounts where the feature is not enabled.
func isFeatureUnavailable(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	for _, s := range []string{
		"not available",
		"not enabled",
		"insufficient permissions",
		"feature",
		"subscription",
		"payment required",
	} {
		if strings.Contains(msg, s) {
			return true
		}
	}
	return false
}
