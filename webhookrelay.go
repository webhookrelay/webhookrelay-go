// Package webhookrelay provides a client library to configure Webhook Relay tunnels, manage serverless functions and set up webhook forwarding routing.
package webhookrelay

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/pkg/errors"
	"golang.org/x/time/rate"
)

const apiURL = "https://my.webhookrelay.com/v1"
const userAgent = "webhookrelay-go/v1"

const (
	// AuthToken specifies that we should authenticate with an API key & secret
	AuthToken = 1 << iota
)

// New creates a new Webhook Relay v1 API client.
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

// API holds the configuration for the current API client. A client should not
// be modified concurrently.
type API struct {
	APIKey    string
	APISecret string
	BaseURL   string
	UserAgent string

	authType    int
	httpClient  *http.Client
	headers     http.Header
	retryPolicy RetryPolicy
	rateLimiter *rate.Limiter
	logger      Logger
}

// newClient provides shared logic
func newClient(opts ...Option) (*API, error) {
	silentLogger := log.New(ioutil.Discard, "", log.LstdFlags)

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
	return api.makeRequestWithAuthType(context.TODO(), method, uri, params, api.authType)
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
			// useful to do some simple logging here, maybe introduce levels later
			api.logger.Printf("Sleeping %s before retry attempt number %d for request %s %s", sleepDuration.String(), i, method, uri)
			time.Sleep(sleepDuration)

		}
		err = api.rateLimiter.Wait(context.TODO())
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
				respBody, err = ioutil.ReadAll(resp.Body)
				resp.Body.Close()

				respErr = errors.Wrap(err, "could not read response body")

				api.logger.Printf("Request: %s %s got an error response %d: %s\n", method, uri, resp.StatusCode,
					strings.Replace(strings.Replace(string(respBody), "\n", "", -1), "\t", "", -1))
			} else {
				api.logger.Printf("Error performing request: %s %s : %s \n", method, uri, respErr.Error())
			}
			continue
		} else {
			respBody, err = ioutil.ReadAll(resp.Body)
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

	switch {
	case resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusMultipleChoices:
	case resp.StatusCode == http.StatusUnauthorized:
		return nil, errors.Errorf("HTTP status %d: invalid credentials", resp.StatusCode)
	case resp.StatusCode == http.StatusForbidden:
		return nil, errors.Errorf("HTTP status %d: insufficient permissions", resp.StatusCode)
	case resp.StatusCode == http.StatusPreconditionFailed:
		return nil, errors.Errorf("HTTP status %d: precondition failed", resp.StatusCode)
	case resp.StatusCode == http.StatusPaymentRequired:
		return nil, errors.Errorf("HTTP status %d: feature not available for your subscription", resp.StatusCode)
	case resp.StatusCode == http.StatusServiceUnavailable,
		resp.StatusCode == http.StatusBadGateway,
		resp.StatusCode == http.StatusGatewayTimeout,
		resp.StatusCode == 522,
		resp.StatusCode == 523,
		resp.StatusCode == 524:
		return nil, errors.Errorf("HTTP status %d: service failure", resp.StatusCode)
	case resp.StatusCode == 400:
		return nil, errors.Errorf("%s", respBody)
	default:
		var s string
		if respBody != nil {
			s = string(respBody)
		}
		return nil, errors.Errorf("HTTP status %d: content %q", resp.StatusCode, s)
	}

	return respBody, nil
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

	req.SetBasicAuth(api.APIKey, api.APISecret)

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
