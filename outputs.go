package webhookrelay

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/pkg/errors"
)

var (
	// ErrNoSuchOutput is the error returned when the Output does not exist.
	ErrNoSuchOutput = errors.New("no such output")
)

// Output specified webhook forwarding destination
type Output struct {
	ID        string    `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Name      string    `json:"name"`
	BucketID  string    `json:"bucket_id"`
	// CronID is set when the output belongs to a cron trigger. readonly
	CronID     string `json:"cron_id"`
	FunctionID string `json:"function_id"`
	// ResponseFunctionID names a function that runs AFTER delivery, once the
	// destination's response (or the final delivery error) is known.
	ResponseFunctionID string              `json:"response_function_id"`
	Headers            map[string][]string `json:"headers"`
	Destination        string              `json:"destination"`
	Disabled           bool                `json:"disabled"` // Allows disabling forwarding to specific output
	// LockPath ensures that the request path cannot be changed from what is
	// specified in the destination. For example if request is coming to /v1/webhooks/xxx/github-jenkins,
	// with lock path 'false' and destination 'http://localhost:8080' it would go to http://localhost:8080/github-jenkins.
	// However, with lock path 'true', it will be sent to 'http://localhost:8080'
	LockPath bool `json:"lock_path"`
	Internal bool `json:"internal"`
	Timeout  int  `json:"timeout"` // Destination response timeout
	// Retries is the number of additional retries after the first 3 attempts.
	// Setting it to -1 disables retries.
	Retries int `json:"retries"`
	// TLSVerification controls TLS certificate verification for public webhooks.
	TLSVerification bool `json:"tls_verification"`
	// Rules optionally gates forwarding to this output based on the incoming request.
	Rules       *Rules `json:"rules,omitempty"`
	Description string `json:"description"`
	CreatedBy   string `json:"created_by"` // readonly
	// Durability configures durable long-period retries for this output.
	Durability *DurabilityConfig `json:"durability,omitempty"`
	// Throttle configures per-output throughput throttling.
	Throttle *ThrottleConfig `json:"throttle,omitempty"`
}

// DurabilityConfig configures durable long-period retries for an output. When
// Enabled, deliveries that keep failing past HandoffAfter are persisted and
// retried by the dispatcher using Schedule until Deadline.
type DurabilityConfig struct {
	// Enabled toggles durable delivery for this output.
	Enabled bool `json:"enabled"`
	// Schedule names a backoff preset: "seconds", "medium", "long" or "custom".
	// Empty defaults to "long" when enabled. Custom backoffs use CustomDelays.
	Schedule string `json:"schedule,omitempty"`
	// CustomDelays is consulted only when Schedule == "custom". Encoded as
	// nanoseconds on the wire (Go time.Duration).
	CustomDelays []time.Duration `json:"custom_delays,omitempty"`
	// Deadline caps total retry duration measured from the webhook's received_at.
	// Zero falls through to a schedule-derived default.
	Deadline time.Duration `json:"deadline,omitempty"`
	// HandoffAfter is the attempt-age threshold past which a failing delivery is
	// moved to the durable retry path. Zero defaults to 15 minutes.
	HandoffAfter time.Duration `json:"handoff_after,omitempty"`
}

// Throttle modes for ThrottleConfig.Mode.
const (
	ThrottleModeRate        = "rate"
	ThrottleModeConcurrency = "concurrency"
)

// Throttle rate intervals for ThrottleConfig.Interval.
const (
	ThrottleIntervalSecond = "second"
	ThrottleIntervalMinute = "minute"
	ThrottleIntervalHour   = "hour"
)

// ThrottleConfig configures per-output throughput throttling. When Enabled,
// webhooks are queued and delivered at the configured rate (ThrottleModeRate)
// or in-flight concurrency cap (ThrottleModeConcurrency).
type ThrottleConfig struct {
	// Enabled toggles throttling for this output.
	Enabled bool `json:"enabled"`
	// Mode is ThrottleModeRate or ThrottleModeConcurrency.
	Mode string `json:"mode,omitempty"`
	// Rate is the maximum deliveries per Interval (ThrottleModeRate only).
	Rate int `json:"rate,omitempty"`
	// Interval is ThrottleIntervalSecond/Minute/Hour (ThrottleModeRate only).
	Interval string `json:"interval,omitempty"`
	// MaxConcurrent is the max in-flight deliveries (ThrottleModeConcurrency only).
	MaxConcurrent int `json:"max_concurrent,omitempty"`
	// MaxQueueDepth bounds the per-output queue (0 = unbounded). When exceeded,
	// new webhooks are rejected with HTTP 429.
	MaxQueueDepth int `json:"max_queue_depth,omitempty"`
	// Deadline caps how long a queued webhook may wait before being dropped.
	// Zero falls through to the server default (24h).
	Deadline time.Duration `json:"deadline,omitempty"`
}

// Rules describes a forwarding rule tree attached to an output. Exactly one of
// the fields is set per node.
type Rules struct {
	And   *AndRule   `json:"and,omitempty"`
	Or    *OrRule    `json:"or,omitempty"`
	Not   *NotRule   `json:"not,omitempty"`
	Match *MatchRule `json:"match,omitempty"`
}

// AndRule evaluates to true only if all child rules are true.
type AndRule []Rules

// OrRule evaluates to true if any child rule is true.
type OrRule []Rules

// NotRule negates its child rule.
type NotRule struct {
	And   *AndRule   `json:"and,omitempty"`
	Or    *OrRule    `json:"or,omitempty"`
	Match *MatchRule `json:"match,omitempty"`
}

// MatchRule matches a request based on Type (e.g. "value", "regex",
// "payload-hash-sha256", "ip-whitelist").
type MatchRule struct {
	Type      string   `json:"type,omitempty"`
	Regex     string   `json:"regex,omitempty"`
	Substring string   `json:"substring,omitempty"`
	Secret    string   `json:"secret,omitempty"`
	Value     string   `json:"value,omitempty"`
	Parameter Argument `json:"parameter,omitempty"`
	IPRange   string   `json:"ip-range,omitempty"`
}

// Argument references a value in the incoming request (header, query, payload, ...).
type Argument struct {
	Source       string `json:"source,omitempty"`
	Name         string `json:"name,omitempty"`
	EnvName      string `json:"envname,omitempty"`
	Base64Decode bool   `json:"base64decode,omitempty"`
}

// MarshalJSON helper to change time into unix
func (o *Output) MarshalJSON() ([]byte, error) {
	type Alias Output
	return json.Marshal(&struct {
		CreatedAt int64 `json:"created_at"`
		UpdatedAt int64 `json:"updated_at"`
		*Alias
	}{
		CreatedAt: o.CreatedAt.Unix(),
		UpdatedAt: o.UpdatedAt.Unix(),
		Alias:     (*Alias)(o),
	})
}

// UnmarshalJSON helper to change time from unix or RFC3339 string
func (o *Output) UnmarshalJSON(data []byte) error {
	type Alias Output
	aux := &struct {
		CreatedAt json.RawMessage `json:"created_at"`
		UpdatedAt json.RawMessage `json:"updated_at"`
		*Alias
	}{
		Alias: (*Alias)(o),
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	var err error
	o.CreatedAt, err = parseTime(aux.CreatedAt)
	if err != nil {
		return err
	}
	o.UpdatedAt, err = parseTime(aux.UpdatedAt)
	if err != nil {
		return err
	}
	return nil
}

// OutputListOptions used to query outputs
type OutputListOptions struct {
	Bucket string // Bucket reference - ID or name
}

// ListOutputs returns a list of outputs belonging to the bucket. If bucket reference not supplied,
// all account outputs will be returned
func (api *API) ListOutputs(options *OutputListOptions) ([]*Output, error) {
	if options.Bucket == "" {
		return api.allOutputList(&BucketListOptions{})
	}

	bucket, err := api.GetBucket(options.Bucket)
	if err != nil {
		return nil, err
	}

	var outputs []*Output
	for idx := range bucket.Outputs {
		outputs = append(outputs, bucket.Outputs[idx])
	}

	return outputs, nil
}

func (api *API) allOutputList(options *BucketListOptions) ([]*Output, error) {
	buckets, err := api.ListBuckets(options)
	if err != nil {
		return nil, fmt.Errorf("failed to get outputs, error: %w", err)
	}

	var outputs []*Output
	for idx := range buckets {
		for bIdx := range buckets[idx].Outputs {
			outputs = append(outputs, buckets[idx].Outputs[bIdx])
		}
	}

	return outputs, nil
}

// CreateOutput creates an Output and returns the new object
func (api *API) CreateOutput(options *Output) (*Output, error) {
	bucketID, err := api.ensureBucketID(options.BucketID)
	if err != nil {
		return nil, err
	}

	resp, err := api.makeRequest("POST", "/buckets/"+bucketID+"/outputs", options)
	if err != nil {
		return nil, err
	}

	var result Output
	err = json.Unmarshal(resp, &result)
	return &result, nil
}

// UpdateOutput updates output
func (api *API) UpdateOutput(options *Output) (*Output, error) {

	bucketID, err := api.ensureBucketID(options.BucketID)
	if err != nil {
		return nil, err
	}

	outputID, err := api.ensureOutputID(bucketID, options.ID)
	if err != nil {
		return nil, err
	}

	resp, err := api.makeRequest("PUT", "/buckets/"+bucketID+"/outputs/"+outputID, options)
	if err != nil {
		return nil, err
	}

	var output Output
	err = json.Unmarshal(resp, &output)
	return &output, nil
}

// OutputDeleteOptions delete options
type OutputDeleteOptions struct {
	Bucket string
	Output string // ID or name
}

// DeleteOutput deletes output from the bucket
func (api *API) DeleteOutput(options *OutputDeleteOptions) error {

	if options.Bucket == "" {
		return fmt.Errorf("bucket not specified")
	}

	if options.Output == "" {
		return fmt.Errorf("output not specified")
	}

	bucketID, err := api.ensureBucketID(options.Bucket)
	if err != nil {
		return err
	}

	outputID, err := api.ensureOutputID(bucketID, options.Output)
	if err != nil {
		return err
	}

	_, err = api.makeRequest("DELETE", "/buckets/"+bucketID+"/outputs/"+outputID, nil)
	return err
}

func (api *API) ensureOutputID(bucket, ref string) (string, error) {
	if !IsUUID(ref) {
		id, err := api.outputIDFromName(bucket, ref)
		if err != nil {
			return "", err
		}
		return id, nil
	}
	return ref, nil
}

func (api *API) outputIDFromName(bucket, name string) (id string, err error) {
	outputs, err := api.ListOutputs(&OutputListOptions{
		Bucket: bucket,
	})
	if err != nil {
		return
	}
	for _, b := range outputs {
		if b.Name == name {
			return b.ID, nil
		}
	}
	return "", ErrNoSuchOutput
}
