package webhookrelay

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// TestCronUnmarshalUnixTimestamps verifies every Cron time field decodes from
// unix seconds (as well as RFC3339, covered by TestCronJSONRoundTrip).
func TestCronUnmarshalUnixTimestamps(t *testing.T) {
	const data = `{"id":"c1","name":"n","created_at":1700000000,"updated_at":1700000050,` +
		`"starts_at":1700000100,"ends_at":1700000200,"next_run":1700000300}`

	var c Cron
	if err := json.Unmarshal([]byte(data), &c); err != nil {
		t.Fatalf("unmarshal unix: %v", err)
	}
	if c.CreatedAt.Unix() != 1700000000 || c.UpdatedAt.Unix() != 1700000050 {
		t.Errorf("created/updated not parsed: %v %v", c.CreatedAt, c.UpdatedAt)
	}
	if c.StartsAt.Unix() != 1700000100 || c.EndsAt.Unix() != 1700000200 {
		t.Errorf("starts/ends not parsed: %v %v", c.StartsAt, c.EndsAt)
	}
	if c.NextRun.Unix() != 1700000300 {
		t.Errorf("next_run not parsed: %v", c.NextRun)
	}
}

// TestCronMarshalOmitsReadOnlyAndZeroBounds verifies CreateCron/UpdateCron send
// a clean payload: read-only fields and zero time bounds are omitted rather than
// serialized as year-1 timestamps or an empty bucket object.
func TestCronMarshalOmitsReadOnlyAndZeroBounds(t *testing.T) {
	c := &Cron{
		Name:        "nightly",
		Schedule:    "@daily",
		Timezone:    "UTC",
		Method:      "POST",
		Destination: "https://example.com/hook",
	}
	data, err := json.Marshal(c)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	s := string(data)
	for _, k := range []string{"bucket", "next_run", "starts_at", "ends_at", "created_at", "updated_at"} {
		if strings.Contains(s, `"`+k+`"`) {
			t.Errorf("expected %q to be omitted, got: %s", k, s)
		}
	}
	if !strings.Contains(s, `"schedule":"@daily"`) || !strings.Contains(s, `"destination":"https://example.com/hook"`) {
		t.Errorf("writable fields missing: %s", s)
	}
}

// TestCronMarshalIncludesSetBounds verifies a non-zero StartsAt is emitted as
// RFC3339 while a still-zero EndsAt stays omitted.
func TestCronMarshalIncludesSetBounds(t *testing.T) {
	start := time.Date(2023, 11, 14, 22, 13, 20, 0, time.UTC)
	c := &Cron{Name: "n", StartsAt: start}

	data, err := json.Marshal(c)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	s := string(data)
	if !strings.Contains(s, `"starts_at":"2023-11-14T22:13:20Z"`) {
		t.Errorf("starts_at not emitted as RFC3339: %s", s)
	}
	if strings.Contains(s, `"ends_at"`) {
		t.Errorf("zero ends_at should be omitted: %s", s)
	}

	// And it round-trips back.
	var got Cron
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !got.StartsAt.Equal(start) {
		t.Errorf("starts_at round-trip: got %v want %v", got.StartsAt, start)
	}
	if !got.EndsAt.IsZero() {
		t.Errorf("ends_at should be zero, got %v", got.EndsAt)
	}
}

// TestServiceConnectionTypesUnmarshalUnixTimestamps verifies the service
// connection resources tolerate unix-second timestamps.
func TestServiceConnectionTypesUnmarshalUnixTimestamps(t *testing.T) {
	t.Run("service_connection", func(t *testing.T) {
		const data = `{"id":"scn_1","created_at":1700000000,"updated_at":1700000050,"last_checked":1700000100}`
		var sc ServiceConnection
		if err := json.Unmarshal([]byte(data), &sc); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if sc.CreatedAt.Unix() != 1700000000 || sc.UpdatedAt.Unix() != 1700000050 || sc.LastChecked.Unix() != 1700000100 {
			t.Errorf("timestamps not parsed: %+v", sc)
		}
	})

	t.Run("service_connection_input", func(t *testing.T) {
		const data = `{"id":"in_1","created_at":1700000000,"updated_at":1700000050}`
		var in ServiceConnectionInput
		if err := json.Unmarshal([]byte(data), &in); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if in.CreatedAt.Unix() != 1700000000 || in.UpdatedAt.Unix() != 1700000050 {
			t.Errorf("timestamps not parsed: %+v", in)
		}
	})

	t.Run("service_connection_output", func(t *testing.T) {
		const data = `{"id":"out_1","created_at":1700000000,"updated_at":1700000050}`
		var out ServiceConnectionOutput
		if err := json.Unmarshal([]byte(data), &out); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if out.CreatedAt.Unix() != 1700000000 || out.UpdatedAt.Unix() != 1700000050 {
			t.Errorf("timestamps not parsed: %+v", out)
		}
	})
}

// TestVariableUnmarshalUnixTimestamps verifies function config variables also
// tolerate unix-second timestamps (RFC3339 is covered by TestVariableRFC3339Timestamps).
func TestVariableUnmarshalUnixTimestamps(t *testing.T) {
	const data = `{"key":"A","value":"1","created_at":1700000000,"updated_at":1700000050}`
	var v Variable
	if err := json.Unmarshal([]byte(data), &v); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if v.CreatedAt.Unix() != 1700000000 || v.UpdatedAt.Unix() != 1700000050 {
		t.Errorf("timestamps not parsed: %+v", v)
	}
	if v.Key != "A" || v.Value != "1" {
		t.Errorf("fields not preserved: %+v", v)
	}
}
