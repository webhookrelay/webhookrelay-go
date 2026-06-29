package webhookrelay

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestCronJSONRoundTrip(t *testing.T) {
	in := &Cron{
		ID:          "cron-1",
		Name:        "nightly",
		Enabled:     true,
		Schedule:    "0 0 * * *",
		Timezone:    "UTC",
		Method:      "POST",
		Payload:     `{"hello":"world"}`,
		Headers:     map[string][]string{"Content-Type": {"application/json"}},
		Destination: "https://example.com/hook",
	}

	data, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var out Cron
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if out.Name != in.Name || out.Schedule != in.Schedule || out.Method != in.Method ||
		out.Destination != in.Destination || out.Payload != in.Payload {
		t.Fatalf("scalar fields not preserved: %+v", out)
	}
	if got := out.Headers["Content-Type"]; len(got) != 1 || got[0] != "application/json" {
		t.Fatalf("headers not preserved: %#v", out.Headers)
	}
}

func TestIntegrationConfigurationUnixTimestamps(t *testing.T) {
	ts := time.Unix(1700000000, 0)
	in := &IntegrationConfiguration{
		ID:            "int-1",
		CreatedAt:     ts,
		UpdatedAt:     ts,
		Plugin:        IntegrationPluginSlack,
		Events:        []string{NotificationEventForwardingDeliveryError},
		Configuration: map[string]interface{}{"webhook_url": "https://hooks.slack.com/services/x"},
	}

	data, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	// Backend encodes integration timestamps as unix seconds, not RFC3339.
	if !strings.Contains(string(data), `"created_at":1700000000`) {
		t.Fatalf("expected unix created_at, got: %s", data)
	}

	var out IntegrationConfiguration
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !out.CreatedAt.Equal(ts) {
		t.Fatalf("created_at not preserved: got %v want %v", out.CreatedAt, ts)
	}
	if out.Plugin != IntegrationPluginSlack || len(out.Events) != 1 {
		t.Fatalf("fields not preserved: %+v", out)
	}

	// The unmarshaller must also tolerate an RFC3339 string (parseTime fallback).
	rfc := `{"created_at":"2023-11-14T22:13:20Z","updated_at":"2023-11-14T22:13:20Z","plugin":"webhook"}`
	var out2 IntegrationConfiguration
	if err := json.Unmarshal([]byte(rfc), &out2); err != nil {
		t.Fatalf("unmarshal rfc3339: %v", err)
	}
	if out2.CreatedAt.IsZero() {
		t.Fatalf("rfc3339 created_at not parsed")
	}
}

func TestServiceConnectionOutputOmitemptySubConfigs(t *testing.T) {
	in := &ServiceConnectionOutput{
		ID:                          "out-1",
		Name:                        "to-discord",
		BucketID:                    "bucket-1",
		ServiceConnectionOutputType: ServiceConnectionOutputTypeDiscord,
		DiscordOutput:               &DiscordOutput{WebhookURL: "https://discord.com/api/webhooks/1/abc"},
	}

	data, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	s := string(data)
	if !strings.Contains(s, `"discord_output"`) {
		t.Fatalf("discord_output missing: %s", s)
	}
	// Unset pointer sub-configs must be omitted.
	for _, unexpected := range []string{"slack_output", "aws_s3_output", "gcp_pubsub_output"} {
		if strings.Contains(s, unexpected) {
			t.Fatalf("expected %q to be omitted: %s", unexpected, s)
		}
	}

	var out ServiceConnectionOutput
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.DiscordOutput == nil || out.DiscordOutput.WebhookURL != in.DiscordOutput.WebhookURL {
		t.Fatalf("discord output not preserved: %+v", out.DiscordOutput)
	}
	if out.SlackOutput != nil {
		t.Fatalf("slack output should be nil")
	}
}

func TestServiceConnectionInputEmail(t *testing.T) {
	in := &ServiceConnectionInput{
		ID:                         "in-1",
		BucketID:                   "bucket-1",
		ServiceConnectionInputType: ServiceConnectionInputTypeEmail,
		EmailInput: EmailInput{
			Enabled:        true,
			AllowedSenders: []string{"alerts@example.com"},
		},
	}

	data, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	// EmailAddress is read-only/computed and omitted when empty.
	if strings.Contains(string(data), "email_address") {
		t.Fatalf("email_address should be omitted when empty: %s", data)
	}

	var out ServiceConnectionInput
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !out.EmailInput.Enabled || len(out.EmailInput.AllowedSenders) != 1 {
		t.Fatalf("email input not preserved: %+v", out.EmailInput)
	}
}

func TestOutputDurabilityThrottleRulesRoundTrip(t *testing.T) {
	in := &Output{
		ID:              "o-1",
		Name:            "durable",
		BucketID:        "bucket-1",
		Destination:     "https://example.com",
		Retries:         -1,
		TLSVerification: true,
		Durability: &DurabilityConfig{
			Enabled:      true,
			Schedule:     "long",
			Deadline:     30 * 24 * time.Hour,
			HandoffAfter: 15 * time.Minute,
		},
		Throttle: &ThrottleConfig{
			Enabled:  true,
			Mode:     ThrottleModeRate,
			Rate:     10,
			Interval: ThrottleIntervalMinute,
		},
		Rules: &Rules{
			Match: &MatchRule{Type: "value", Value: "ping", Parameter: Argument{Source: "header", Name: "X-Event"}},
		},
	}

	data, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var out Output
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if out.Retries != -1 || !out.TLSVerification {
		t.Fatalf("scalar fields not preserved: %+v", out)
	}
	if out.Durability == nil || out.Durability.Schedule != "long" ||
		out.Durability.Deadline != 30*24*time.Hour || out.Durability.HandoffAfter != 15*time.Minute {
		t.Fatalf("durability not preserved: %+v", out.Durability)
	}
	if out.Throttle == nil || out.Throttle.Mode != ThrottleModeRate || out.Throttle.Rate != 10 ||
		out.Throttle.Interval != ThrottleIntervalMinute {
		t.Fatalf("throttle not preserved: %+v", out.Throttle)
	}
	if out.Rules == nil || out.Rules.Match == nil || out.Rules.Match.Value != "ping" ||
		out.Rules.Match.Parameter.Name != "X-Event" {
		t.Fatalf("rules not preserved: %+v", out.Rules)
	}
}

func TestServiceConnectionRoundTrip(t *testing.T) {
	in := &ServiceConnection{
		ID:          "scn_123",
		Name:        "prod-gcp",
		ServiceType: ServiceGCP,
		Status:      ServiceConnectionStatusConnected,
		GCPServiceConnection: GCPServiceConnection{
			ProjectID:         "my-project",
			ServiceAccountKey: "secret",
		},
	}

	data, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var out ServiceConnection
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.ServiceType != ServiceGCP || out.GCPServiceConnection.ProjectID != "my-project" {
		t.Fatalf("service connection not preserved: %+v", out)
	}
}
