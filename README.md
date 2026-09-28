# Webhook Relay Go client

[![GoDoc](https://img.shields.io/badge/godoc-reference-5673AF.svg?style=flat-square)](https://pkg.go.dev/github.com/webhookrelay/webhookrelay-go)
[![Tests](https://github.com/webhookrelay/webhookrelay-go/actions/workflows/test.yml/badge.svg)](https://github.com/webhookrelay/webhookrelay-go/actions/workflows/test.yml)

Go client for the [Webhook Relay](https://webhookrelay.com) v1 API.

Webhook Relay receives webhooks and HTTP requests on a public endpoint and delivers
them wherever you need — public URLs, services **behind a firewall or on localhost**,
**WebSocket** clients, or on a **schedule** — with in-flight **transformations**,
**durable retries**, **throttling** and fine-grained routing. This library configures
all of it from Go.

## Installation

```shell
go get github.com/webhookrelay/webhookrelay-go
```

Requires a working [Go](https://golang.org/) environment (Go 1.23+).

## Authentication

Generate credentials [here](https://my.webhookrelay.com/tokens). There are two ways to authenticate:

- **API key (recommended)** — a single key (looks like `sk-...`) sent as a Bearer token:

  ```go
  api, err := webhookrelay.NewWithAPIKey(os.Getenv("RELAY_API_KEY"))
  ```

- **Key & secret pair** — sent as HTTP Basic auth:

  ```go
  api, err := webhookrelay.New(os.Getenv("RELAY_KEY"), os.Getenv("RELAY_SECRET"))
  ```

## Quickstart

Create a bucket (which comes with a ready-to-use public input endpoint) and forward
everything it receives to a destination:

```go
package main

import (
	"fmt"
	"log"
	"os"

	"github.com/webhookrelay/webhookrelay-go"
)

func main() {
	api, err := webhookrelay.NewWithAPIKey(os.Getenv("RELAY_API_KEY"))
	if err != nil {
		log.Fatal(err)
	}

	// A bucket groups an input (public URL) with one or more outputs (destinations).
	bucket, err := api.CreateBucket(&webhookrelay.BucketCreateOptions{
		Name: "sendgrid-to-segment",
	})
	if err != nil {
		log.Fatal(err)
	}

	// Every bucket gets a default input. Point your webhook provider at this URL:
	fmt.Println("Send webhooks to:", bucket.Inputs[0].EndpointURL())

	// Forward everything the bucket receives to a public destination.
	if _, err := api.CreateOutput(&webhookrelay.Output{
		BucketID:    bucket.ID,
		Name:        "segment",
		Destination: "https://webhooks.segment.com?b=yyyy",
	}); err != nil {
		log.Fatal(err)
	}
}
```

## Features

### Internal webhooks — deliver behind a firewall

Mark an output as `Internal` (or just point it at a private/localhost address) and
it will only be delivered by the [`relay`](https://webhookrelay.com/v1/installation/)
agent running inside your network. This lets an external provider reach a service
that has no public IP.

```go
// Public providers deliver to bucket.Inputs[0].EndpointURL(); the agent picks the
// webhooks up and forwards them to your internal service.
_, err = api.CreateOutput(&webhookrelay.Output{
	BucketID:    bucket.ID,
	Name:        "local-dev",
	Destination: "http://localhost:8080/webhooks",
	Internal:    true, // relayed by the agent, not the public forwarder
})
```

Then run the agent to open the connection:

```shell
relay forward --bucket sendgrid-to-segment http://localhost:8080/webhooks
```

### WebSocket streaming

Enable streaming on a bucket to receive its webhooks in real time over a WebSocket
(`wss://my.webhookrelay.com/v1/socket`) — handy for browsers, dashboards, or any
client that can't expose an endpoint.

```go
bucket, err := api.GetBucket("sendgrid-to-segment")
if err != nil {
	log.Fatal(err)
}
bucket.Stream = true
if _, err := api.UpdateBucket(bucket); err != nil {
	log.Fatal(err)
}
```

When acting as a WebSocket "transponder" (your client, not Webhook Relay, produces
the HTTP response), send the response back within 10 seconds using `UpdateWebhookLog`:

```go
err = api.UpdateWebhookLog(&webhookrelay.WebhookLogsUpdateRequest{
	ID:         logID, // the webhook's log ID, received over the socket
	StatusCode: 200,
	Status:     webhookrelay.RequestStatusSent,
	ResponseBody: []byte(`{"ok":true}`),
})
```

### Polling for webhooks

Prefer to pull instead of being pushed? Poll a bucket's logs and process new
webhooks yourself. `BucketID` is required; filter by status, time range and paginate.

```go
res, err := api.ListWebhookLogs(&webhookrelay.WebhookLogsListOptions{
	BucketID: bucket.ID,
	Status:   webhookrelay.RequestStatusReceived,
	Limit:    50,
})
if err != nil {
	log.Fatal(err)
}
for _, entry := range res.Data {
	fmt.Printf("%s %s -> %d\n", entry.Method, entry.ID, entry.StatusCode)
	// entry.Body, entry.Headers, entry.RawQuery ...
}
```

### Durable delivery

If a destination is down, durable delivery persists failing webhooks and keeps
retrying on a backoff schedule until they succeed or hit a deadline — so an outage
doesn't lose events.

```go
import "time"

_, err = api.CreateOutput(&webhookrelay.Output{
	BucketID:    bucket.ID,
	Name:        "billing",
	Destination: "https://api.internal.example.com/webhooks",
	Durability: &webhookrelay.DurabilityConfig{
		Enabled:      true,
		Schedule:     "long",           // "seconds", "medium", "long" or "custom"
		Deadline:     30 * 24 * time.Hour, // give up after 30 days
		HandoffAfter: 15 * time.Minute,    // move to durable retries after 15m of failures
	},
})
```

### Throttling

Cap how fast webhooks are delivered to a fragile or rate-limited destination.
Queue and release at a fixed **rate**, or bound **concurrency**.

```go
// Rate limit: at most 10 deliveries per minute.
_, err = api.CreateOutput(&webhookrelay.Output{
	BucketID:    bucket.ID,
	Name:        "rate-limited-api",
	Destination: "https://api.example.com/webhooks",
	Throttle: &webhookrelay.ThrottleConfig{
		Enabled:  true,
		Mode:     webhookrelay.ThrottleModeRate,
		Rate:     10,
		Interval: webhookrelay.ThrottleIntervalMinute,
	},
})

// Or bound in-flight concurrency instead:
throttle := &webhookrelay.ThrottleConfig{
	Enabled:       true,
	Mode:          webhookrelay.ThrottleModeConcurrency,
	MaxConcurrent: 5,
	MaxQueueDepth: 1000, // reject with HTTP 429 once the queue is full
}
_ = throttle
```

### Replay missing internal deliveries when an agent reconnects

Recover never-attempted deliveries that arrived while an internal agent was
disconnected. The server scans at most `Limit` recent rows inside `Lookback`.

```go
_, err = api.CreateOutput(&webhookrelay.Output{
	BucketID:    bucket.ID,
	Name:        "cluster-agent",
	Destination: "http://webhook-handler.default.svc:8080/webhooks",
	Internal:    true,
	ReplayMissing: &webhookrelay.ReplayMissingConfig{
		Enabled:  true,
		Lookback: 30 * time.Minute,
		Limit:    250,
	},
})
```

### Outbound webhooks — send webhooks to your customers

Register each customer as a consumer with the HTTPS endpoint they give you, then
publish events. Webhook Relay signs every delivery
([Standard Webhooks](https://www.standardwebhooks.com/)), retries failures durably
for up to 48 hours and keeps the delivery history. Outbound webhooks are in pilot:
the account needs the `outbound` feature.

```go
// Once per customer.
_, err = api.UpsertOutboundEventType(&webhookrelay.OutboundEventTypeOptions{Name: "invoice.paid"})
_, err = api.UpsertOutboundConsumer(&webhookrelay.OutboundConsumerOptions{ID: "customer_42", Name: "Acme"})
endpoint, err := api.CreateOutboundEndpoint(&webhookrelay.OutboundEndpointOptions{
	Consumer:   "customer_42",
	URL:        "https://customer.example/webhooks",
	EventTypes: []string{"invoice.paid"},
})
// endpoint.Secret is the signing secret your customer verifies deliveries with.

// Every time the event happens.
message, err := api.PublishOutboundMessage(&webhookrelay.OutboundMessagePublishOptions{
	Consumer:       "customer_42",
	EventType:      "invoice.paid",
	Payload:        map[string]interface{}{"invoice_id": "inv_123", "amount": 4900},
	IdempotencyKey: "invoice-paid:inv_123", // retrying with the same key never publishes twice
})
```

Publishing is asynchronous: the returned message is the durable acceptance
receipt. `GetOutboundMessage` shows each endpoint's delivery and attempts;
`RetryOutboundDelivery`, `RecoverOutboundDeliveries` and
`ReplayMissingOutboundDeliveries` re-send in the background and return a
recovery task. Without an `IdempotencyKey` the client generates one per call,
so its own automatic retries are safe.

Watch endpoint health across all your customers. Endpoints carry `Stats`
(attempts and failures over the last 24 hours) when listed or read:

```go
health, err := api.GetOutboundHealth()
// health.Endpoints["failing"], health.Stats.Attempts, health.Stats.Failures

// Failing endpoints come longest failing first. Filter by Consumer too;
// Limit is 1-100 (0 means 50), page with Offset.
failing, err := api.ListAllOutboundEndpoints(&webhookrelay.OutboundEndpointListOptions{
	State: webhookrelay.OutboundEndpointStateFailing,
	Limit: 20,
})
```

A runnable version lives in [`examples/outbound`](examples/outbound).

## More capabilities

The client covers the rest of the Webhook Relay API too:

- **Transformation functions** (JavaScript / Lua) to reshape webhooks in flight —
  `CreateFunction`, `SetFunctionConfigurationVariable`, `InvokeFunction`. Attach one
  to an input or output via `FunctionID`.
- **Recurring webhooks** on a cron schedule — `CreateCron`.
- **Tunnels** to expose a local service over a public hostname — `CreateTunnel`.
- **Service connections** (GCP / AWS / Azure) plus cloud event **inputs** (Pub/Sub,
  S3, SQS, SNS) and **outputs** (Pub/Sub, GCS, S3, SQS, SNS, Discord, Slack).
- **Email-to-webhook** — an inbound email address that relays mail through a bucket
  (`CreateServiceConnectionInput` with type `email`).
- **Notification integrations** — `CreateIntegration`, `AddIntegrationBucket`.
- **Routing rules**, retries, TLS controls, custom domains, and webhook logs.

## Client options

`New` / `NewWithAPIKey` accept functional options:

```go
api, err := webhookrelay.NewWithAPIKey(os.Getenv("RELAY_API_KEY"),
	webhookrelay.WithUserAgent("my-app/1.2.3"),
	webhookrelay.WithRetryPolicy(3, 1, 30), // retries, min/max backoff (seconds)
)
```

Available options: `WithHTTPClient`, `WithAPIEndpointURL`, `WithHeaders`,
`WithRetryPolicy`, `WithUserAgent`, `WithLogger`.

## License

See [LICENSE](LICENSE).
