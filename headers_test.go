package webhookrelay

import (
	"encoding/json"
	"testing"
)

// The exact payload a live account returns for GET /v1/crons when the schedule
// was created in the dashboard. Before HeaderValues this failed with
// "cannot unmarshal string into Go struct field .Alias.headers of type
// []string", taking down ListCrons, GetCron and everything built on them.
const dashboardCron = `[{
  "id": "812d5723-606d-4e44-a1cc-6741b8697eba",
  "created_at": "2025-11-30T10:48:37.443926Z",
  "updated_at": "2026-06-10T19:10:04.545298Z",
  "name": "Regular Health Check",
  "enabled": true,
  "recurring": true,
  "schedule": "*/5 * * * *",
  "timezone": "Europe/London",
  "method": "POST",
  "payload": "{\"regular\":\"check\"}",
  "headers": {"Authorization": "Bearer redacted"},
  "starts_at": "0001-01-01T00:00:00Z",
  "ends_at": "0001-01-01T00:00:00Z",
  "next_run": "2026-09-02T18:20:00Z"
}]`

func TestCronWithDashboardWrittenHeadersDecodes(t *testing.T) {
	var crons []*Cron
	if err := json.Unmarshal([]byte(dashboardCron), &crons); err != nil {
		t.Fatalf("decoding a cron created in the dashboard: %v", err)
	}
	if len(crons) != 1 {
		t.Fatalf("got %d crons, want 1", len(crons))
	}
	if got := crons[0].Headers.Get("Authorization"); got != "Bearer redacted" {
		t.Errorf("Authorization = %q, want %q", got, "Bearer redacted")
	}
	if crons[0].Schedule != "*/5 * * * *" {
		t.Errorf("schedule = %q", crons[0].Schedule)
	}
}

func TestHeaderValuesAcceptsBothShapes(t *testing.T) {
	for _, tc := range []struct {
		name, input string
		want        map[string][]string
	}{
		{"scalar", `{"X-Token":"abc"}`, map[string][]string{"X-Token": {"abc"}}},
		{"list", `{"X-Token":["abc"]}`, map[string][]string{"X-Token": {"abc"}}},
		{"multi", `{"X-Token":["a","b"]}`, map[string][]string{"X-Token": {"a", "b"}}},
		{"mixed", `{"A":"1","B":["2","3"]}`, map[string][]string{"A": {"1"}, "B": {"2", "3"}}},
		{"empty object", `{}`, map[string][]string{}},
		{"null", `null`, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var got HeaderValues
			if err := json.Unmarshal([]byte(tc.input), &got); err != nil {
				t.Fatalf("unmarshal %s: %v", tc.input, err)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("got %#v, want %#v", got, tc.want)
			}
			for name, want := range tc.want {
				values := got[name]
				if len(values) != len(want) {
					t.Fatalf("%s: got %#v, want %#v", name, values, want)
				}
				for i := range want {
					if values[i] != want[i] {
						t.Errorf("%s[%d] = %q, want %q", name, i, values[i], want[i])
					}
				}
			}
		})
	}
}

// A header the API could not have written is a bug worth reporting clearly:
// the message names the header, because the alternative is a decode failure
// against a list of a hundred crons with no clue which one caused it.
func TestHeaderValuesRejectsUnusableValuesByName(t *testing.T) {
	var got HeaderValues
	err := json.Unmarshal([]byte(`{"X-Ok":"fine","X-Bad":{"nested":true}}`), &got)
	if err == nil {
		t.Fatal("a nested object was accepted as a header value")
	}
	if !contains(err.Error(), "X-Bad") {
		t.Errorf("error does not name the offending header: %v", err)
	}
}

// Encoding always emits the list form, so a value read from the API and
// written back keeps its meaning whichever shape it arrived in.
func TestHeaderValuesRoundTripToTheListForm(t *testing.T) {
	var decoded HeaderValues
	if err := json.Unmarshal([]byte(`{"X-Token":"abc"}`), &decoded); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(decoded)
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) != `{"X-Token":["abc"]}` {
		t.Errorf("re-encoded as %s, want the list form", encoded)
	}
}

func TestHeaderValuesGetIsSafeOnMissingAndEmpty(t *testing.T) {
	h := HeaderValues{"X-Empty": {}}
	if got := h.Get("X-Empty"); got != "" {
		t.Errorf("empty header returned %q", got)
	}
	if got := h.Get("X-Absent"); got != "" {
		t.Errorf("absent header returned %q", got)
	}
	var nilHeaders HeaderValues
	if got := nilHeaders.Get("anything"); got != "" {
		t.Errorf("nil map returned %q", got)
	}
}

// Inputs and outputs read the same opaque store, so a client that wrote the
// scalar form leaves records this one must still be able to read.
func TestInputAndOutputAcceptScalarHeaders(t *testing.T) {
	var in Input
	if err := json.Unmarshal([]byte(`{"id":"i-1","headers":{"X-Api-Key":"secret"}}`), &in); err != nil {
		t.Fatalf("input: %v", err)
	}
	if got := in.Headers.Get("X-Api-Key"); got != "secret" {
		t.Errorf("input header = %q", got)
	}

	var out Output
	if err := json.Unmarshal([]byte(`{"id":"o-1","headers":{"X-Api-Key":"secret"}}`), &out); err != nil {
		t.Fatalf("output: %v", err)
	}
	if got := out.Headers.Get("X-Api-Key"); got != "secret" {
		t.Errorf("output header = %q", got)
	}
}

func contains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
