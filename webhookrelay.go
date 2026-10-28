// Package webhookrelay provides a client library to configure Webhook Relay tunnels, manage serverless functions and set up webhook forwarding routing.
package webhookrelay

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"
	"golang.org/x/time/rate"
)

const apiURL = "https://my.webhookrelay.com/v1"
const userAgent = "webhookrelay-go/v1"

const (
	// AuthToken specifies that we should authenticate with an API key & secret
	// pair, sent as HTTP Basic auth.
	AuthToken = 1 << iota
	// AuthAPIKey specifies that we should authenticate with a single API key,
	// sent as an "Authorization: Bearer <key>" header. API keys look like
	// "sk-...".
	AuthAPIKey
)

// New creates a new Webhook Relay v1 API client authenticated with an API key
// and secret pair (sent as HTTP Basic auth).
func New(key, secret string, opts ...Option) (*API, error) {
	if key == "" || secret == "" {
		return nil, ErrEmptyCredentials
	}

	api, err := newClient(opts...)
	if err != nil {
		return nil, err
	}

	api.APIKey = key
	api.APISecret = secret

	return api, nil
}

// NewWithAPIKey creates a new Webhook Relay v1 API client authenticated with a
// single API key. The key is sent as an "Authorization: Bearer <key>" header.
// This is the simplest way to authenticate — generate an API key (looks like
// "sk-...") at https://my.webhookrelay.com/tokens.
func NewWithAPIKey(apiKey string, opts ...Option) (*API, error) {
	if apiKey == "" {
		return nil, ErrEmptyCredentials
	}

	api, err := newClient(opts...)
	if err != nil {
		return nil, err
	}

	api.APIToken = apiKey
	api.authType = AuthAPIKey

	return api, nil
}

// API holds the configuration for the current API client. A client should not
// be modified concurrently.
type API struct {
	APIKey    string
	APISecret string
	// APIToken is a single API key used for Bearer authentication (set via
	// NewWithAPIKey). When non-empty it takes precedence over APIKey/APISecret.
	APIToken  string
	BaseURL   string
	UserAgent string

	authType    int
	httpClient  *http.Client
	headers     http.Header
	retryPolicy RetryPolicy
	rateLimiter *rate.Limiter
	logger      Logger
	// reqCtx is the per-call context set via WithContext; nil means
	// context.Background(). It bounds rate-limiter waits, retry backoff and
	// the HTTP requests themselves.
	reqCtx context.Context
}

// WithContext returns a shallow copy of the client whose requests are bound
// to ctx: cancelling it aborts rate-limiter waits, retry backoff sleeps and
// in-flight HTTP requests. The copy shares the underlying HTTP client, rate
// limiter and configuration, so it is cheap to create per call:
//
//	buckets, err := api.WithContext(ctx).ListBuckets(&BucketListOptions{})
func (api *API) WithContext(ctx context.Context) *API {
	clone := *api
	clone.reqCtx = ctx
	return &clone
}

// requestContext resolves the context requests run under.
func (api *API) requestContext() context.Context {
	if api.reqCtx != nil {
		return api.reqCtx
	}
	return context.Background()
}

// newClient provides shared logic
func newClient(opts ...Option) (*API, error) {
	silentLogger := log.New(io.Discard, "", log.LstdFlags)

	api := &API{
		BaseURL:     apiURL,
		headers:     make(http.Header),
		authType:    AuthToken,
		UserAgent:   userAgent,
		rateLimiter: rate.NewLimiter(rate.Limit(4), 1), // 4rps equates to default api limit (1200 req/5 min)
		retryPolicy: RetryPolicy{
			MaxRetries:    3,
			MinRetryDelay: time.Duration(1) * time.Second,
			MaxRetryDelay: time.Duration(30) * time.Second,
		},
		logger: silentLogger,
	}

	err := api.parseOptions(opts...)
	if err != nil {
		return nil, fmt.Errorf("options parsing failed: %w", err)
	}

	// Fall back to http.DefaultClient if the package user does not provide
	// their own.
	if api.httpClient == nil {
		api.httpClient = http.DefaultClient
	}

	return api, nil
}

func (api *API) Host() string {
	u, err := url.Parse(api.BaseURL)
	if err != nil {
		return ""
	}
	host, _, _ := net.SplitHostPort(u.Host)
	if host == "" {
		return u.Host
	}
	return host
}

func (api *API) Scheme() string {
	if strings.HasPrefix(api.BaseURL, "https://") {
		return "https"
	}
	return "http"
}

func (api *API) Endpoint() string {
	return api.BaseURL
}

// RetryPolicy specifies number of retries and min/max retry delays
// This config is used when the client exponentially backs off after errored requests
type RetryPolicy struct {
	MaxRetries    int
	MinRetryDelay time.Duration
	MaxRetryDelay time.Duration
}

// Logger defines the interface this library needs to use logging
// This is a subset of the methods implemented in the log package
type Logger interface {
	Printf(format string, v ...interface{})
}

// makeRequest makes a HTTP request and returns the body as a byte slice,
// closing it before returning. params will be serialized to JSON.
func (api *API) makeRequest(method, uri string, params interface{}) ([]byte, error) {
	return api.makeRequestWithAuthType(api.requestContext(), method, uri, params, api.authType)
}

func (api *API) makeRequestContext(ctx context.Context, method, uri string, params interface{}) ([]byte, error) {
	return api.makeRequestWithAuthType(ctx, method, uri, params, api.authType)
}

func (api *API) makeRequestWithAuthType(ctx context.Context, method, uri string, params interface{}, authType int) ([]byte, error) {
	return api.makeRequestWithAuthTypeAndHeaders(ctx, method, uri, params, authType, nil)
}

func (api *API) makeRequestWithAuthTypeAndHeaders(ctx context.Context, method, uri string, params interface{}, authType int, headers http.Header) ([]byte, error) {
	// Replace nil with a JSON object if needed
	var jsonBody []byte
	var err error

	if params != nil {
		if paramBytes, ok := params.([]byte); ok {
			jsonBody = paramBytes
		} else {
			jsonBody, err = json.Marshal(params)
			if err != nil {
				return nil, errors.Wrap(err, "error marshalling params to JSON")
			}
		}
	} else {
		jsonBody = nil
	}

	var resp *http.Response
	var respErr error
	var reqBody io.Reader
	var respBody []byte
	// retryAfter is the server's Retry-After on the last response, which
	// replaces the backoff for the next attempt.
	var retryAfter time.Duration
	for i := 0; i <= api.retryPolicy.MaxRetries; i++ {
		if jsonBody != nil {
			reqBody = bytes.NewReader(jsonBody)
		}
		if i > 0 {
			// expect the backoff introduced here on errored requests to dominate the effect of rate limiting
			// don't need a random component here as the rate limiter should do something similar
			// nb time duration could truncate an arbitrary float. Since our inputs are all ints, we should be ok
			sleepDuration := time.Duration(math.Pow(2, float64(i-1)) * float64(api.retryPolicy.MinRetryDelay))

			if sleepDuration > api.retryPolicy.MaxRetryDelay {
				sleepDuration = api.retryPolicy.MaxRetryDelay
			}
			if retryAfter > 0 {
				sleepDuration = retryAfter
			}
			// useful to do some simple logging here, maybe introduce levels later
			api.logger.Printf("Sleeping %s before retry attempt number %d for request %s %s", sleepDuration.String(), i, method, uri)
			// A cancelled caller should not sit out the backoff.
			select {
			case <-time.After(sleepDuration):
			case <-ctx.Done():
				return nil, ctx.Err()
			}

		}
		// Waiting on the caller's context, not a background one: without this
		// a cancelled request would still queue behind the rate limiter.
		err = api.rateLimiter.Wait(ctx)
		if err != nil {
			return nil, errors.Wrap(err, "Error caused by request rate limiting")
		}
		resp, respErr = api.request(ctx, method, uri, reqBody, authType, headers)

		// retry if the server is rate limiting us or if it failed
		// assumes server operations are rolled back on failure
		if respErr != nil || resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			// if we got a valid http response, try to read body so we can reuse the connection
			// see https://golang.org/pkg/net/http/#Client.Do
			if respErr == nil {
				respBody, err = io.ReadAll(resp.Body)
				resp.Body.Close()

				respErr = errors.Wrap(err, "could not read response body")

				api.logger.Printf("Request: %s %s got an error response %d: %s\n", method, uri, resp.StatusCode,
					strings.Replace(strings.Replace(string(respBody), "\n", "", -1), "\t", "", -1))

				var retry bool
				retry, retryAfter = api.shouldRetry(resp, respBody)
				if !retry {
					break
				}
			} else {
				api.logger.Printf("Error performing request: %s %s : %s \n", method, uri, respErr.Error())
			}
			continue
		} else {
			respBody, err = io.ReadAll(resp.Body)
			defer resp.Body.Close()
			if err != nil {
				return nil, errors.Wrap(err, "could not read response body")
			}
			break
		}
	}
	if respErr != nil {
		return nil, respErr
	}

	if resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusMultipleChoices {
		return respBody, nil
	}
	return nil, newAPIError(resp.StatusCode, resp.Header, respBody)
}

// shouldRetry decides whether a 429 or 5xx response is worth retrying, and
// how long to wait first when the server said so (zero means the policy's
// backoff). A 429 for an exhausted quota, or one whose Retry-After is beyond
// the policy's max retry delay, would fail again within the retries, so it
// is returned to the caller straight away.
func (api *API) shouldRetry(resp *http.Response, body []byte) (bool, time.Duration) {
	if resp.StatusCode != http.StatusTooManyRequests {
		return true, 0
	}
	if isQuotaExhausted(body) {
		return false, 0
	}
	header := resp.Header.Get("Retry-After")
	if header == "" {
		return true, 0
	}
	wait, ok := parseRetryAfter(header, time.Now())
	if !ok {
		return true, 0
	}
	if wait > api.retryPolicy.MaxRetryDelay {
		return false, 0
	}
	return true, wait
}

// isQuotaExhausted reports whether a 429 body is one that lasts until the
// next billing period: the Outbound Gateway's monthly message quota
// ({"error": "monthly outbound message quota reached: ..."}) or the agent's
// monthly token allowance ({"code": "agent_quota_exceeded"}).
func isQuotaExhausted(body []byte) bool {
	var payload struct {
		Error string `json:"error"`
		Code  string `json:"code"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return false
	}
	return payload.Code == "agent_quota_exceeded" ||
		strings.HasPrefix(payload.Error, "monthly outbound message quota reached")
}

// parseRetryAfter reads a Retry-After header given in seconds or as an HTTP
// date.
func parseRetryAfter(header string, now time.Time) (time.Duration, bool) {
	if secs, err := strconv.Atoi(header); err == nil {
		if secs < 0 {
			return 0, false
		}
		return time.Duration(secs) * time.Second, true
	}
	at, err := http.ParseTime(header)
	if err != nil {
		return 0, false
	}
	if wait := at.Sub(now); wait > 0 {
		return wait, true
	}
	return 0, true
}

// request makes a HTTP request to the given API endpoint, returning the raw
// *http.Response, or an error if one occurred. The caller is responsible for
// closing the response body.
func (api *API) request(ctx context.Context, method, uri string, reqBody io.Reader, authType int, headers http.Header) (*http.Response, error) {
	req, err := http.NewRequest(method, api.BaseURL+uri, reqBody)
	if err != nil {
		return nil, errors.Wrap(err, "HTTP request creation failed")
	}
	req = req.WithContext(ctx)

	combinedHeaders := make(http.Header)
	copyHeader(combinedHeaders, api.headers)
	copyHeader(combinedHeaders, headers)
	req.Header = combinedHeaders

	if authType == AuthAPIKey && api.APIToken != "" {
		req.Header.Set("Authorization", "Bearer "+api.APIToken)
	} else {
		req.SetBasicAuth(api.APIKey, api.APISecret)
	}

	if api.UserAgent != "" {
		req.Header.Set("User-Agent", api.UserAgent)
	}

	if req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := api.httpClient.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "HTTP request failed")
	}

	return resp, nil
}

// copyHeader copies all headers for `source` and sets them on `target`.
// based on https://godoc.org/github.com/golang/gddo/httputil/header#Copy
func copyHeader(target, source http.Header) {
	for k, vs := range source {
		target[k] = vs
	}
}
